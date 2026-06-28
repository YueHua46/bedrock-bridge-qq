package doctor

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"mcqq-bridge/internal/config"
	"mcqq-bridge/internal/store"
)

type Item struct {
	Name       string
	OK         bool
	Detail     string
	Suggestion string
}

type Report struct {
	OK    bool
	Items []Item
}

func Run(root, cfgPath string) (Report, error) {
	var report Report
	add := func(name string, ok bool, detail, suggestion string) {
		report.Items = append(report.Items, Item{Name: name, OK: ok, Detail: detail, Suggestion: suggestion})
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		add("config file", false, err.Error(), "Run mcqq-bridge init, then edit data/config.yml.")
		report.OK = false
		return report, nil
	}
	add("config file", true, cfgPath, "")
	add("minecraft token", cfg.Minecraft.Token != "", "configured", "Regenerate data/config.yml or set minecraft.token.")

	dbPath := filepath.Join(root, "data", "database.sqlite")
	db, err := store.Open(dbPath)
	if err != nil {
		add("sqlite database", false, err.Error(), "Check whether data/ is writable.")
	} else {
		defer db.Close()
		add("sqlite database", db.Ping() == nil, dbPath, "Check whether data/database.sqlite is locked or unwritable.")
		if hb, ok, err := db.LastHeartbeat(cfg.Minecraft.ServerID); err == nil && ok {
			fresh := time.Since(hb.UpdatedAt) < 60*time.Second
			add("minecraft heartbeat", fresh, fmt.Sprintf("last seen %s, online %d", hb.UpdatedAt.Local().Format(time.DateTime), hb.OnlinePlayers), "Check behavior pack, bridgeUrl, token, and BDS Script API settings.")
		} else {
			add("minecraft heartbeat", false, "not received", "Start BDS with the generated behavior pack enabled.")
		}
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Server.Port), 800*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		add("bridge http port", true, fmt.Sprintf("127.0.0.1:%d", cfg.Server.Port), "")
	} else {
		add("bridge http port", false, err.Error(), "Run mcqq-bridge start before checking live HTTP status.")
	}

	if _, err := os.Stat(filepath.Join(root, "logs")); err == nil {
		add("logs directory", true, filepath.Join(root, "logs"), "")
	} else {
		add("logs directory", false, err.Error(), "Create logs/ or rerun mcqq-bridge init.")
	}

	report.OK = true
	for _, item := range report.Items {
		if !item.OK {
			report.OK = false
			break
		}
	}
	return report, nil
}
