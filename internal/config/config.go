package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server" json:"server"`
	Minecraft MinecraftConfig `yaml:"minecraft" json:"minecraft"`
	QQ        QQConfig        `yaml:"qq" json:"qq"`
	OneBot    OneBotConfig    `yaml:"onebot" json:"onebot"`
	Features  FeaturesConfig  `yaml:"features" json:"features"`
	Security  SecurityConfig  `yaml:"security" json:"security"`
}

type ServerConfig struct {
	Host      string `yaml:"host" json:"host"`
	Port      int    `yaml:"port" json:"port"`
	PublicURL string `yaml:"public_url" json:"public_url"`
}

type MinecraftConfig struct {
	ServerID               string `yaml:"server_id" json:"server_id"`
	Token                  string `yaml:"token" json:"token"`
	PollIntervalTicks      int    `yaml:"poll_interval_ticks" json:"poll_interval_ticks"`
	HeartbeatIntervalTicks int    `yaml:"heartbeat_interval_ticks" json:"heartbeat_interval_ticks"`
	EnableCommandQueue     bool   `yaml:"enable_command_queue" json:"enable_command_queue"`
}

type QQConfig struct {
	GroupID       int64   `yaml:"group_id" json:"group_id"`
	ForwardPrefix string  `yaml:"forward_prefix" json:"forward_prefix"`
	AdminQQ       []int64 `yaml:"admin_qq" json:"admin_qq"`
}

type OneBotConfig struct {
	Provider    string `yaml:"provider" json:"provider"`
	Mode        string `yaml:"mode" json:"mode"`
	WSURL       string `yaml:"ws_url" json:"ws_url"`
	HTTPURL     string `yaml:"http_url" json:"http_url"`
	AccessToken string `yaml:"access_token" json:"access_token"`
}

type FeaturesConfig struct {
	MCToQQChat      bool `yaml:"mc_to_qq_chat" json:"mc_to_qq_chat"`
	QQToMCChat      bool `yaml:"qq_to_mc_chat" json:"qq_to_mc_chat"`
	JoinLeaveNotice bool `yaml:"join_leave_notice" json:"join_leave_notice"`
	DeathNotice     bool `yaml:"death_notice" json:"death_notice"`
	OnlineCommand   bool `yaml:"online_command" json:"online_command"`
	BindCommand     bool `yaml:"bind_command" json:"bind_command"`
}

type SecurityConfig struct {
	MaxMessageLength   int  `yaml:"max_message_length" json:"max_message_length"`
	RateLimitPerMinute int  `yaml:"rate_limit_per_minute" json:"rate_limit_per_minute"`
	AllowQQCommands    bool `yaml:"allow_qq_commands" json:"allow_qq_commands"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Host:      "0.0.0.0",
			Port:      8080,
			PublicURL: "http://127.0.0.1:8080",
		},
		Minecraft: MinecraftConfig{
			ServerID:               "survival",
			Token:                  randomToken(),
			PollIntervalTicks:      40,
			HeartbeatIntervalTicks: 200,
			EnableCommandQueue:     true,
		},
		QQ: QQConfig{
			GroupID:       123456789,
			ForwardPrefix: "",
			AdminQQ:       []int64{},
		},
		OneBot: OneBotConfig{
			Provider:    "napcat",
			Mode:        "websocket",
			WSURL:       "ws://127.0.0.1:3001",
			HTTPURL:     "http://127.0.0.1:3000",
			AccessToken: randomToken(),
		},
		Features: FeaturesConfig{
			MCToQQChat:      true,
			QQToMCChat:      true,
			JoinLeaveNotice: true,
			DeathNotice:     false,
			OnlineCommand:   true,
			BindCommand:     false,
		},
		Security: SecurityConfig{
			MaxMessageLength:   200,
			RateLimitPerMinute: 30,
			AllowQQCommands:    false,
		},
	}
}

func LoadOrCreate(path string) (Config, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		return cfg, Save(path, cfg)
	}
	return Load(path)
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	Normalize(&cfg)
	return cfg, Validate(cfg)
}

func Save(path string, cfg Config) error {
	Normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func Normalize(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.PublicURL == "" {
		cfg.Server.PublicURL = "http://127.0.0.1:8080"
	}
	if cfg.Minecraft.ServerID == "" {
		cfg.Minecraft.ServerID = "survival"
	}
	if cfg.Minecraft.Token == "" {
		cfg.Minecraft.Token = randomToken()
	}
	if cfg.Minecraft.PollIntervalTicks == 0 {
		cfg.Minecraft.PollIntervalTicks = 40
	}
	if cfg.Minecraft.HeartbeatIntervalTicks == 0 {
		cfg.Minecraft.HeartbeatIntervalTicks = 200
	}
	if cfg.OneBot.Provider == "" {
		cfg.OneBot.Provider = "napcat"
	}
	if cfg.OneBot.Mode == "" {
		cfg.OneBot.Mode = "websocket"
	}
	if cfg.OneBot.WSURL == "" {
		cfg.OneBot.WSURL = "ws://127.0.0.1:3001"
	}
	if cfg.OneBot.HTTPURL == "" {
		cfg.OneBot.HTTPURL = "http://127.0.0.1:3000"
	}
	if cfg.OneBot.AccessToken == "" {
		cfg.OneBot.AccessToken = randomToken()
	}
	if cfg.Security.MaxMessageLength == 0 {
		cfg.Security.MaxMessageLength = 200
	}
	if cfg.Security.RateLimitPerMinute == 0 {
		cfg.Security.RateLimitPerMinute = 30
	}
}

func Validate(cfg Config) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}
	if _, err := url.ParseRequestURI(cfg.Server.PublicURL); err != nil {
		return errors.New("server.public_url must be a valid URL")
	}
	if cfg.Minecraft.Token == "" {
		return errors.New("minecraft.token is required")
	}
	if cfg.Minecraft.ServerID == "" {
		return errors.New("minecraft.server_id is required")
	}
	if cfg.Security.MaxMessageLength < 20 {
		return errors.New("security.max_message_length must be at least 20")
	}
	return nil
}

func Set(cfg *Config, key, value string) error {
	switch key {
	case "server.host":
		cfg.Server.Host = value
	case "server.port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("server.port must be a number")
		}
		cfg.Server.Port = port
	case "server.public_url":
		cfg.Server.PublicURL = value
	case "minecraft.server_id":
		cfg.Minecraft.ServerID = value
	case "minecraft.token":
		cfg.Minecraft.Token = value
	case "minecraft.poll_interval_ticks":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("minecraft.poll_interval_ticks must be a number")
		}
		cfg.Minecraft.PollIntervalTicks = n
	case "qq.group_id":
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("qq.group_id must be a number")
		}
		cfg.QQ.GroupID = id
	case "qq.forward_prefix":
		cfg.QQ.ForwardPrefix = value
	case "onebot.ws_url":
		cfg.OneBot.WSURL = value
	case "onebot.http_url":
		cfg.OneBot.HTTPURL = value
	case "onebot.access_token":
		cfg.OneBot.AccessToken = value
	case "features.mc_to_qq_chat":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("features.mc_to_qq_chat must be true or false")
		}
		cfg.Features.MCToQQChat = v
	case "features.qq_to_mc_chat":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("features.qq_to_mc_chat must be true or false")
		}
		cfg.Features.QQToMCChat = v
	case "security.max_message_length":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("security.max_message_length must be a number")
		}
		cfg.Security.MaxMessageLength = n
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	Normalize(cfg)
	return Validate(*cfg)
}

func randomToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "change-me-token"
	}
	return hex.EncodeToString(buf)
}
