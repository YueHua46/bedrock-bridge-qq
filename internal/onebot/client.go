package onebot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	WSURL       string
	HTTPURL     string
	AccessToken string
	GroupID     int64
	Prefix      string
}

type GroupMessage struct {
	GroupID    int64  `json:"group_id"`
	UserID     int64  `json:"user_id"`
	Nickname   string `json:"nickname"`
	RawMessage string `json:"raw_message"`
	MessageID  int64  `json:"message_id"`
}

type Client struct {
	cfg       Config
	logger    *slog.Logger
	http      *http.Client
	onMessage func(GroupMessage)
	connected atomic.Bool
	mu        sync.Mutex
	conn      *websocket.Conn
}

func New(cfg Config, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		cfg:    cfg,
		logger: logger,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) OnGroupMessage(fn func(GroupMessage)) {
	c.onMessage = fn
}

func (c *Client) Connected() bool {
	return c.connected.Load()
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.connected.Store(false)
}

func (c *Client) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if c.cfg.WSURL == "" {
			time.Sleep(10 * time.Second)
			continue
		}
		if err := c.connectOnce(ctx); err != nil && ctx.Err() == nil {
			c.logger.Warn("onebot websocket disconnected", "error", err)
			c.connected.Store(false)
			time.Sleep(5 * time.Second)
		}
	}
}

func (c *Client) connectOnce(ctx context.Context) error {
	u, err := url.Parse(c.cfg.WSURL)
	if err != nil {
		return err
	}
	header := http.Header{}
	if c.cfg.AccessToken != "" {
		header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), header)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.mu.Unlock()
		_ = conn.Close()
		c.connected.Store(false)
	}()

	c.connected.Store(true)
	c.logger.Info("onebot websocket connected", "url", c.cfg.WSURL)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		c.handleEvent(data)
	}
}

func (c *Client) SendGroupMessage(ctx context.Context, groupID int64, message string) error {
	if c.cfg.HTTPURL != "" {
		return c.sendViaHTTP(ctx, groupID, message)
	}
	return c.sendViaWS(groupID, message)
}

func (c *Client) sendViaHTTP(ctx context.Context, groupID int64, message string) error {
	endpoint := strings.TrimRight(c.cfg.HTTPURL, "/") + "/send_group_msg"
	payload := map[string]any{
		"group_id": groupID,
		"message":  message,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("onebot http status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func (c *Client) sendViaWS(groupID int64, message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("onebot websocket is not connected")
	}
	payload := map[string]any{
		"action": "send_group_msg",
		"params": map[string]any{
			"group_id": groupID,
			"message":  message,
		},
		"echo": fmt.Sprintf("mcqq-%d", time.Now().UnixNano()),
	}
	return c.conn.WriteJSON(payload)
}

func (c *Client) GetLoginInfo(ctx context.Context) (map[string]any, error) {
	if c.cfg.HTTPURL == "" {
		if c.Connected() {
			return map[string]any{"connected": true}, nil
		}
		return nil, fmt.Errorf("onebot websocket is not connected")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.cfg.HTTPURL, "/")+"/get_login_info", nil)
	if err != nil {
		return nil, err
	}
	if c.cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("onebot status %d", resp.StatusCode)
	}
	var data map[string]any
	return data, json.NewDecoder(resp.Body).Decode(&data)
}

func (c *Client) handleEvent(data []byte) {
	var ev struct {
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
	if err := json.Unmarshal(data, &ev); err != nil {
		return
	}
	if ev.PostType != "message" || ev.MessageType != "group" {
		return
	}
	if c.cfg.GroupID != 0 && ev.GroupID != c.cfg.GroupID {
		return
	}
	nick := ev.Sender.Card
	if nick == "" {
		nick = ev.Sender.Nickname
	}
	if nick == "" {
		nick = fmt.Sprintf("%d", ev.UserID)
	}
	if c.onMessage != nil {
		c.onMessage(GroupMessage{
			GroupID:    ev.GroupID,
			UserID:     ev.UserID,
			Nickname:   nick,
			RawMessage: ev.RawMessage,
			MessageID:  ev.MessageID,
		})
	}
}
