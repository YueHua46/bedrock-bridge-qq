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

const (
	redacted = "<redacted>"

	maxConfigBody = 64 * 1024  // data/config.yml payload upper bound
	maxMCBody     = 32 * 1024  // /api/mc/* event/ack/heartbeat
	maxOneBotBody = 256 * 1024 // /onebot/event may carry forwarded media URLs
	maxAckBatch   = 500        // /api/mc/ack ids per request
)

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
	mux.HandleFunc("/api/status", a.requireAdmin(a.handleStatus))
	mux.HandleFunc("/api/setup/save", a.requireAdmin(a.handleSaveConfig))
	mux.HandleFunc("/api/logs/recent", a.requireAdmin(a.handleRecentLogs))
	mux.HandleFunc("/api/pack/download", a.requireAdmin(a.handlePackDownload))
	mux.HandleFunc("/api/onebot/test", a.requireAdmin(a.handleOneBotTest))
	mux.HandleFunc("/api/mc/events", a.handleMCEvents)
	mux.HandleFunc("/api/mc/pull", a.handleMCPull)
	mux.HandleFunc("/api/mc/ack", a.handleMCAck)
	mux.HandleFunc("/api/mc/heartbeat", a.handleMCHeartbeat)
	mux.HandleFunc("/onebot/event", a.requireOneBot(a.handleOneBotEvent))
}

// requireAdmin guards endpoints that mutate Bridge state or expose secrets
// (config, pack, logs, OneBot test, full status). The token is
// security.admin_token. Because browsers cannot set a custom Authorization
// header in a CSRF form/POST and cannot read cross-origin responses to fetch
// the token, this also defeats CSRF without needing a separate token.
func (a *App) requireAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !security.BearerOK(r, a.cfg.Security.AdminToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// requireOneBot guards the OneBot HTTP callback. NapCat sends the configured
// access_token as a Bearer header, which proves the request really came from
// the OneBot implementation and prevents third parties from injecting forged
// group messages into Minecraft.
func (a *App) requireOneBot(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !security.BearerOK(r, a.cfg.OneBot.AccessToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/setup", http.StatusFound)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "version": a.opt.Version})
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	hb, hasHB, _ := a.opt.Store.LastHeartbeat(a.cfg.Minecraft.ServerID)
	// Return a copy of the config with all long-lived secrets redacted so the
	// status endpoint never leaks credentials through logs or proxies.
	cfgOut := a.cfg
	cfgOut.Minecraft.Token = redacted
	cfgOut.OneBot.AccessToken = redacted
	cfgOut.Security.AdminToken = redacted
	writeJSON(w, map[string]any{
		"version":          a.opt.Version,
		"onebot_connected": a.onebot.Connected(),
		"config":           cfgOut,
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
	r.Body = http.MaxBytesReader(w, r.Body, maxConfigBody)
	var next config.Config
	if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
		http.Error(w, "invalid config body", http.StatusBadRequest)
		return
	}
	// Preserve any secrets the UI did not echo back, so a stale page cannot
	// silently blank admin/onebot/minecraft tokens.
	if next.Security.AdminToken == "" {
		next.Security.AdminToken = a.cfg.Security.AdminToken
	}
	if next.OneBot.AccessToken == "" {
		next.OneBot.AccessToken = a.cfg.OneBot.AccessToken
	}
	if next.Minecraft.Token == "" {
		next.Minecraft.Token = a.cfg.Minecraft.Token
	}
	config.Normalize(&next)
	if err := config.Save(a.opt.ConfigPath, next); err != nil {
		http.Error(w, "config save failed", http.StatusBadRequest)
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
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMCBody)
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
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if ev.ServerID != a.cfg.Minecraft.ServerID {
		http.Error(w, "server_id mismatch", http.StatusBadRequest)
		return
	}
	key := "mc:" + ev.Player.Name
	if !a.mcLimiter.Allow(key) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	first, err := a.opt.Store.MarkTrace(ev.TraceID, "mc")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !first {
		writeJSON(w, map[string]any{"ok": true, "duplicate": true})
		return
	}

	msg := security.CleanMessage(ev.Message, a.cfg.Security.MaxMessageLength)
	if msg == "" && ev.Type == "chat" {
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
			http.Error(w, "send to QQ failed", http.StatusBadGateway)
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleMCPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !security.BearerOK(r, a.cfg.Minecraft.Token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	serverID := r.URL.Query().Get("server_id")
	if serverID == "" {
		serverID = a.cfg.Minecraft.ServerID
	}
	if serverID != a.cfg.Minecraft.ServerID {
		http.Error(w, "server_id mismatch", http.StatusBadRequest)
		return
	}
	msgs, err := a.opt.Store.Pull(serverID, 50)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
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
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMCBody)
	var req struct {
		ServerID string   `json:"server_id"`
		IDs      []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.ServerID != a.cfg.Minecraft.ServerID {
		http.Error(w, "server_id mismatch", http.StatusBadRequest)
		return
	}
	if len(req.IDs) > maxAckBatch {
		http.Error(w, "too many ids", http.StatusBadRequest)
		return
	}
	if err := a.opt.Store.Ack(req.ServerID, req.IDs); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
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
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMCBody)
	var req struct {
		ServerID      string `json:"server_id"`
		OnlinePlayers int    `json:"online_players"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.ServerID != a.cfg.Minecraft.ServerID {
		http.Error(w, "server_id mismatch", http.StatusBadRequest)
		return
	}
	if req.OnlinePlayers < 0 || req.OnlinePlayers > 100000 {
		http.Error(w, "invalid online_players", http.StatusBadRequest)
		return
	}
	if err := a.opt.Store.SaveHeartbeat(req.ServerID, req.OnlinePlayers); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleOneBotEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOneBotBody)
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
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if raw.PostType == "message" && raw.MessageType == "group" {
		nick := raw.Sender.Card
		if nick == "" {
			nick = raw.Sender.Nickname
		}
		msg := onebot.GroupMessage{
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

func logMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).String())
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
