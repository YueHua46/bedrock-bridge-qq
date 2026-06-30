package napcat

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"

	"mcqq-bridge/internal/config"
)

type oneBotConfig struct {
	Network             networkConfig `json:"network"`
	MusicSignURL        string        `json:"musicSignUrl"`
	EnableLocalFile2URL bool          `json:"enableLocalFile2Url"`
	ParseMultMsg        bool          `json:"parseMultMsg"`
}

type networkConfig struct {
	HTTPServers      []httpServer      `json:"httpServers"`
	HTTPClients      []any             `json:"httpClients"`
	WebSocketServers []webSocketServer `json:"websocketServers"`
	WebSocketClients []any             `json:"websocketClients"`
}

type httpServer struct {
	Enable            bool   `json:"enable"`
	Name              string `json:"name"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	EnableCors        bool   `json:"enableCors"`
	EnableWebSocket   bool   `json:"enableWebsocket"`
	MessagePostFormat string `json:"messagePostFormat"`
	Token             string `json:"token"`
	Debug             bool   `json:"debug"`
}

type webSocketServer struct {
	Enable            bool   `json:"enable"`
	Name              string `json:"name"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	MessagePostFormat string `json:"messagePostFormat"`
	Token             string `json:"token"`
	Debug             bool   `json:"debug"`
}

func WriteConfig(root string, cfg config.Config) error {
	napcatDir := filepath.Join(root, "napcat")
	if _, err := os.Stat(napcatDir); err != nil {
		return nil
	}

	ob := oneBotConfig{
		Network: networkConfig{
			HTTPServers: []httpServer{{
				Enable:            true,
				Name:              "mcqq-http",
				Host:              "127.0.0.1",
				Port:              portFromURL(cfg.OneBot.HTTPURL, 3000),
				EnableCors:        true,
				EnableWebSocket:   false,
				MessagePostFormat: "string",
				Token:             cfg.OneBot.AccessToken,
				Debug:             false,
			}},
			HTTPClients: []any{},
			WebSocketServers: []webSocketServer{{
				Enable:            true,
				Name:              "mcqq-ws",
				Host:              "127.0.0.1",
				Port:              portFromURL(cfg.OneBot.WSURL, 3001),
				MessagePostFormat: "string",
				Token:             cfg.OneBot.AccessToken,
				Debug:             false,
			}},
			WebSocketClients: []any{},
		},
	}

	targets := []string{
		filepath.Join(napcatDir, "config", "onebot11.json"),
	}
	shells, _ := filepath.Glob(filepath.Join(napcatDir, "NapCat*.Shell"))
	for _, shell := range shells {
		targets = append(targets, filepath.Join(shell, "config", "onebot11.json"))
		accountFiles, _ := filepath.Glob(filepath.Join(shell, "config", "onebot11_*.json"))
		targets = append(targets, accountFiles...)
		nested, _ := filepath.Glob(filepath.Join(shell, "versions", "*", "resources", "app", "napcat", "config"))
		for _, configDir := range nested {
			targets = append(targets, filepath.Join(configDir, "onebot11.json"))
			accountFiles, _ := filepath.Glob(filepath.Join(configDir, "onebot11_*.json"))
			targets = append(targets, accountFiles...)
		}
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if seen[target] {
			continue
		}
		seen[target] = true
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := writeOneBotConfig(target, ob); err != nil {
			return err
		}
	}
	return nil
}

func writeOneBotConfig(target string, ob oneBotConfig) error {
	doc := map[string]any{}
	if data, err := os.ReadFile(target); err == nil {
		_ = json.Unmarshal(data, &doc)
	}

	network, _ := doc["network"].(map[string]any)
	if network == nil {
		network = map[string]any{}
	}
	network["httpServers"] = ob.Network.HTTPServers
	if _, ok := network["httpSseServers"]; !ok {
		network["httpSseServers"] = []any{}
	}
	network["httpClients"] = ob.Network.HTTPClients
	network["websocketServers"] = ob.Network.WebSocketServers
	network["websocketClients"] = ob.Network.WebSocketClients
	if _, ok := network["plugins"]; !ok {
		network["plugins"] = []any{}
	}
	doc["network"] = network

	if _, ok := doc["musicSignUrl"]; !ok {
		doc["musicSignUrl"] = ob.MusicSignURL
	}
	if _, ok := doc["enableLocalFile2Url"]; !ok {
		doc["enableLocalFile2Url"] = ob.EnableLocalFile2URL
	}
	if _, ok := doc["parseMultMsg"]; !ok {
		doc["parseMultMsg"] = ob.ParseMultMsg
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0644)
}

func portFromURL(raw string, fallback int) int {
	u, err := url.Parse(raw)
	if err != nil || u.Port() == "" {
		return fallback
	}
	if u.Port() == "3000" {
		return 3000
	}
	if u.Port() == "3001" {
		return 3001
	}
	var port int
	for _, ch := range u.Port() {
		if ch < '0' || ch > '9' {
			return fallback
		}
		port = port*10 + int(ch-'0')
	}
	if port <= 0 || port > 65535 {
		return fallback
	}
	return port
}
