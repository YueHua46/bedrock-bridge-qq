package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"mcqq-bridge/internal/config"
	"mcqq-bridge/internal/onebot"
	"mcqq-bridge/internal/pack"
	"mcqq-bridge/internal/security"
	"mcqq-bridge/internal/store"
)

type Options struct {
	RootDir     string
	ConfigPath  string
	Config      config.Config
	Store       *store.Store
	Logger      *slog.Logger
	BridgeOnly  bool
	Version     string
	OpenBrowser bool
}

type App struct {
	opt       Options
	cfg       config.Config
	onebot    *onebot.Client
	qqLimiter *security.Limiter
	mcLimiter *security.Limiter
	obMu      sync.Mutex
	obCancel  context.CancelFunc
}

func New(opt Options) (*App, error) {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	cfg := opt.Config
	config.Normalize(&cfg)
	ob := onebot.New(onebot.Config{
		WSURL:       cfg.OneBot.WSURL,
		HTTPURL:     cfg.OneBot.HTTPURL,
		AccessToken: cfg.OneBot.AccessToken,
		GroupID:     cfg.QQ.GroupID,
		Prefix:      cfg.QQ.ForwardPrefix,
	}, opt.Logger)
	app := &App{
		opt:       opt,
		cfg:       cfg,
		onebot:    ob,
		qqLimiter: security.NewLimiter(cfg.Security.RateLimitPerMinute, time.Minute),
		mcLimiter: security.NewLimiter(cfg.Security.RateLimitPerMinute, time.Minute),
	}
	ob.OnGroupMessage(app.handleQQMessage)
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	a.startOneBot(ctx)

	mux := http.NewServeMux()
	a.routes(mux)
	addr := fmt.Sprintf("%s:%d", a.cfg.Server.Host, a.cfg.Server.Port)
	server := &http.Server{Addr: addr, Handler: logMiddleware(a.opt.Logger, mux)}

	errCh := make(chan error, 1)
	go func() {
		a.opt.Logger.Info("bridge http listening", "addr", addr)
		if a.opt.OpenBrowser {
			go func() {
				time.Sleep(600 * time.Millisecond)
				openBrowser(a.cfg.Server.PublicURL + "/setup")
			}()
		}
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (a *App) routes(mux *http.ServeMux) {
	mux.HandleFunc("/", a.handleHome)
	mux.HandleFunc("/setup", a.handleSetup)
	mux.HandleFunc("/status", a.handleStatusPage)
	mux.HandleFunc("/pack", a.handlePackPage)
	mux.HandleFunc("/doctor", a.handleDoctorPage)
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/setup/save", a.handleSaveConfig)
	mux.HandleFunc("/api/logs/recent", a.handleRecentLogs)
	mux.HandleFunc("/api/pack/download", a.handlePackDownload)
	mux.HandleFunc("/api/onebot/test", a.handleOneBotTest)
	mux.HandleFunc("/api/mc/events", a.handleMCEvents)
	mux.HandleFunc("/api/mc/pull", a.handleMCPull)
	mux.HandleFunc("/api/mc/ack", a.handleMCAck)
	mux.HandleFunc("/api/mc/heartbeat", a.handleMCHeartbeat)
	mux.HandleFunc("/onebot/event", a.handleOneBotEvent)
}

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/setup", http.StatusFound)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "version": a.opt.Version})
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	hb, hasHB, _ := a.opt.Store.LastHeartbeat(a.cfg.Minecraft.ServerID)
	writeJSON(w, map[string]any{
		"version":          a.opt.Version,
		"onebot_connected": a.onebot.Connected(),
		"config":           a.cfg,
		"last_heartbeat":   hb,
		"has_heartbeat":    hasHB,
		"server_time":      time.Now(),
	})
}

func (a *App) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var next config.Config
	if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	config.Normalize(&next)
	if err := config.Save(a.opt.ConfigPath, next); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.cfg = next
	a.replaceOneBot(context.Background(), next)
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) startOneBot(parent context.Context) {
	a.replaceOneBot(parent, a.cfg)
}

func (a *App) replaceOneBot(parent context.Context, cfg config.Config) {
	a.obMu.Lock()
	defer a.obMu.Unlock()

	if a.obCancel != nil {
		a.obCancel()
		a.obCancel = nil
	}
	if a.onebot != nil {
		a.onebot.Close()
	}

	ctx, cancel := context.WithCancel(parent)
	a.obCancel = cancel
	a.onebot = onebot.New(onebot.Config{
		WSURL:       cfg.OneBot.WSURL,
		HTTPURL:     cfg.OneBot.HTTPURL,
		AccessToken: cfg.OneBot.AccessToken,
		GroupID:     cfg.QQ.GroupID,
		Prefix:      cfg.QQ.ForwardPrefix,
	}, a.opt.Logger)
	a.onebot.OnGroupMessage(a.handleQQMessage)
	go a.onebot.Run(ctx)
}

func (a *App) handleRecentLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := a.opt.Store.RecentLogs(80)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"logs": logs})
}

func (a *App) handleOneBotTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if err := a.onebot.SendGroupMessage(ctx, a.cfg.QQ.GroupID, "[MCQQ Bridge] 测试消息：Bridge 可以向 QQ 群发送消息。"); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleMCEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !security.BearerOK(r, a.cfg.Minecraft.Token) {
		a.mcAuthError(w)
		return
	}
	var ev struct {
		ServerID string `json:"server_id"`
		Type     string `json:"type"`
		TraceID  string `json:"trace_id"`
		Player   struct {
			Name string `json:"name"`
			XUID string `json:"xuid"`
		} `json:"player"`
		Message string `json:"message"`
		Time    int64  `json:"time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if ev.ServerID != a.cfg.Minecraft.ServerID {
		a.mcServerIDError(w)
		return
	}
	key := "mc:" + ev.Player.Name
	if !a.mcLimiter.Allow(key) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	first, err := a.opt.Store.MarkTrace(ev.TraceID, "mc")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !first {
		writeJSON(w, map[string]any{"ok": true, "duplicate": true})
		return
	}

	msg := security.CleanMessage(ev.Message, a.cfg.Security.MaxMessageLength)
	if msg == "" {
		writeJSON(w, map[string]any{"ok": true, "ignored": true})
		return
	}

	if a.cfg.Features.MCToQQChat {
		text := a.formatMCEvent(ev.Type, ev.Player.Name, msg)
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		err = a.onebot.SendGroupMessage(ctx, a.cfg.QQ.GroupID, text)
		cancel()
		if err != nil {
			a.opt.Store.Log("error", "send to QQ failed: "+err.Error())
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleMCPull(w http.ResponseWriter, r *http.Request) {
	if !security.BearerOK(r, a.cfg.Minecraft.Token) {
		a.mcAuthError(w)
		return
	}
	serverID := r.URL.Query().Get("server_id")
	if serverID == "" {
		serverID = a.cfg.Minecraft.ServerID
	}
	if serverID != a.cfg.Minecraft.ServerID {
		a.mcServerIDError(w)
		return
	}
	msgs, err := a.opt.Store.Pull(serverID, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"messages": msgs})
}

func (a *App) handleMCAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !security.BearerOK(r, a.cfg.Minecraft.Token) {
		a.mcAuthError(w)
		return
	}
	var req struct {
		ServerID string   `json:"server_id"`
		IDs      []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ServerID != a.cfg.Minecraft.ServerID {
		a.mcServerIDError(w)
		return
	}
	if err := a.opt.Store.Ack(req.ServerID, req.IDs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleMCHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !security.BearerOK(r, a.cfg.Minecraft.Token) {
		a.mcAuthError(w)
		return
	}
	var req struct {
		ServerID      string `json:"server_id"`
		OnlinePlayers int    `json:"online_players"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ServerID != a.cfg.Minecraft.ServerID {
		a.mcServerIDError(w)
		return
	}
	if err := a.opt.Store.SaveHeartbeat(req.ServerID, req.OnlinePlayers); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleOneBotEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var msg onebot.GroupMessage
	var raw struct {
		PostType    string `json:"post_type"`
		MessageType string `json:"message_type"`
		GroupID     int64  `json:"group_id"`
		UserID      int64  `json:"user_id"`
		RawMessage  string `json:"raw_message"`
		MessageID   int64  `json:"message_id"`
		Sender      struct {
			Nickname string `json:"nickname"`
			Card     string `json:"card"`
		} `json:"sender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if raw.PostType == "message" && raw.MessageType == "group" {
		nick := raw.Sender.Card
		if nick == "" {
			nick = raw.Sender.Nickname
		}
		msg = onebot.GroupMessage{
			GroupID:    raw.GroupID,
			UserID:     raw.UserID,
			Nickname:   nick,
			RawMessage: raw.RawMessage,
			MessageID:  raw.MessageID,
		}
		a.handleQQMessage(msg)
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handlePackDownload(w http.ResponseWriter, r *http.Request) {
	data, err := pack.Generate(a.cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="mcqq-bridge-behavior-pack.mcpack"`)
	_, _ = w.Write(data)
}

func (a *App) handleQQMessage(msg onebot.GroupMessage) {
	if !a.cfg.Features.QQToMCChat {
		return
	}
	if a.cfg.QQ.GroupID != 0 && msg.GroupID != a.cfg.QQ.GroupID {
		return
	}
	raw := strings.TrimSpace(msg.RawMessage)
	if !strings.HasPrefix(raw, a.cfg.QQ.ForwardPrefix) {
		return
	}
	if !a.qqLimiter.Allow(fmt.Sprintf("qq:%d", msg.UserID)) {
		return
	}
	content := strings.TrimSpace(strings.TrimPrefix(raw, a.cfg.QQ.ForwardPrefix))
	content = onebot.CleanForMinecraft(content)
	content = security.CleanMessage(content, a.cfg.Security.MaxMessageLength)
	if content == "" {
		return
	}
	id := fmt.Sprintf("qq-%d-%d-%d", msg.GroupID, msg.UserID, msg.MessageID)
	if msg.MessageID == 0 {
		id = fmt.Sprintf("qq-%d-%d-%d", msg.GroupID, msg.UserID, time.Now().UnixNano())
	}
	text := fmt.Sprintf("§b[QQ] %s：%s", msg.Nickname, content)
	if err := a.opt.Store.Enqueue(store.MCMessage{
		ID:        id,
		ServerID:  a.cfg.Minecraft.ServerID,
		Type:      "broadcast",
		Text:      text,
		Source:    "qq",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		a.opt.Logger.Error("enqueue qq message failed", "error", err)
	}
}

func (a *App) formatMCEvent(eventType, player, msg string) string {
	switch eventType {
	case "join":
		return fmt.Sprintf("[MC] %s 加入了服务器", player)
	case "leave":
		return fmt.Sprintf("[MC] %s 离开了服务器", player)
	case "death":
		return fmt.Sprintf("[MC] %s %s", player, msg)
	default:
		return fmt.Sprintf("[MC] %s：%s", player, msg)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func (a *App) mcAuthError(w http.ResponseWriter) {
	http.Error(w, "unauthorized: behavior pack token does not match current minecraft.token; regenerate and reinstall the MCQQ Bridge behavior pack", http.StatusUnauthorized)
}

func (a *App) mcServerIDError(w http.ResponseWriter) {
	http.Error(w, "server_id mismatch: behavior pack serverId does not match current minecraft.server_id; regenerate and reinstall the MCQQ Bridge behavior pack", http.StatusBadRequest)
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func logMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		logger.Info("http request", "method", r.Method, "path", r.URL.Path, "status", status, "bytes", rec.bytes, "duration", time.Since(start).String())
	})
}

func openBrowser(target string) {
	if target == "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	_ = cmd.Start()
}

func localIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			return ipNet.IP.String()
		}
	}
	return "127.0.0.1"
}
