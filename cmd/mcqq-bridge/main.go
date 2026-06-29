package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"mcqq-bridge/internal/bridge"
	"mcqq-bridge/internal/config"
	"mcqq-bridge/internal/doctor"
	"mcqq-bridge/internal/logx"
	"mcqq-bridge/internal/napcat"
	"mcqq-bridge/internal/pack"
	"mcqq-bridge/internal/store"
)

const version = "0.1.0"

func main() {
	root, err := os.Getwd()
	if err != nil {
		fatal(err)
	}

	cmd := "start"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "start":
		if err := runStart(root, false); err != nil {
			fatal(err)
		}
	case "bridge-only":
		if err := runStart(root, true); err != nil {
			fatal(err)
		}
	case "init":
		if err := runInit(root); err != nil {
			fatal(err)
		}
	case "config":
		if err := runConfig(root, os.Args[2:]); err != nil {
			fatal(err)
		}
	case "pack":
		if err := runPack(root, os.Args[2:]); err != nil {
			fatal(err)
		}
	case "doctor":
		if err := runDoctor(root); err != nil {
			fatal(err)
		}
	case "version", "--version", "-v":
		fmt.Printf("mcqq-bridge %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func runInit(root string) error {
	cfgPath := filepath.Join(root, "data", "config.yml")
	cfg, err := config.LoadOrCreate(cfgPath)
	if err != nil {
		return err
	}
	if err := napcat.WriteConfig(root, cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0755); err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(root, "data", "database.sqlite"))
	if err != nil {
		return err
	}
	defer db.Close()
	fmt.Println("initialized data/config.yml and data/database.sqlite")
	return nil
}

func runStart(root string, bridgeOnly bool) error {
	cfgPath := filepath.Join(root, "data", "config.yml")
	cfg, err := config.LoadOrCreate(cfgPath)
	if err != nil {
		return err
	}
	if err := napcat.WriteConfig(root, cfg); err != nil {
		return err
	}

	logger, closeLog, err := logx.New(filepath.Join(root, "logs", "mcqq-bridge.log"))
	if err != nil {
		return err
	}
	defer closeLog()

	db, err := store.Open(filepath.Join(root, "data", "database.sqlite"))
	if err != nil {
		return err
	}
	defer db.Close()

	app, err := bridge.New(bridge.Options{
		RootDir:     root,
		ConfigPath:  cfgPath,
		Config:      cfg,
		Store:       db,
		Logger:      logger,
		BridgeOnly:  bridgeOnly,
		Version:     version,
		OpenBrowser: !bridgeOnly,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return app.Run(ctx)
}

func runDoctor(root string) error {
	cfgPath := filepath.Join(root, "data", "config.yml")
	report, err := doctor.Run(root, cfgPath)
	if err != nil {
		return err
	}
	for _, item := range report.Items {
		icon := "x"
		if item.OK {
			icon = "v"
		}
		fmt.Printf("[%s] %s", icon, item.Name)
		if item.Detail != "" {
			fmt.Printf(": %s", item.Detail)
		}
		fmt.Println()
		if !item.OK && item.Suggestion != "" {
			fmt.Printf("    suggestion: %s\n", item.Suggestion)
		}
	}
	if !report.OK {
		os.Exit(1)
	}
	return nil
}

func runConfig(root string, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: mcqq-bridge config show|set <key> <value>")
	}
	cfgPath := filepath.Join(root, "data", "config.yml")
	cfg, err := config.LoadOrCreate(cfgPath)
	if err != nil {
		return err
	}
	switch args[0] {
	case "show":
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	case "set":
		if len(args) < 3 {
			return errors.New("usage: mcqq-bridge config set <key> <value>")
		}
		if err := config.Set(&cfg, args[1], args[2]); err != nil {
			return err
		}
		if err := config.Save(cfgPath, cfg); err != nil {
			return err
		}
		if err := napcat.WriteConfig(root, cfg); err != nil {
			return err
		}
		fmt.Printf("updated %s\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown config command: %s", args[0])
	}
}

func runPack(root string, args []string) error {
	if len(args) == 0 || args[0] != "generate" {
		return errors.New("usage: mcqq-bridge pack generate [output.mcpack]")
	}
	out := filepath.Join(root, "mcqq-bridge-behavior-pack.mcpack")
	if len(args) >= 2 {
		out = args[1]
	}
	cfg, err := config.LoadOrCreate(filepath.Join(root, "data", "config.yml"))
	if err != nil {
		return err
	}
	data, err := pack.Generate(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(out, data, 0600); err != nil {
		return err
	}
	fmt.Printf("behavior pack written: %s\n", out)
	return nil
}

func printHelp() {
	fmt.Println(`MCQQ Bridge

Usage:
  mcqq-bridge start        Start HTTP service and OneBot connector
  mcqq-bridge bridge-only  Start without opening setup page
  mcqq-bridge init         Create default config and database
  mcqq-bridge config show  Print data/config.yml
  mcqq-bridge config set <key> <value>
                           Update config without Web UI
  mcqq-bridge pack generate [output.mcpack]
                           Generate behavior pack without Web UI
  mcqq-bridge doctor       Run local diagnostics
  mcqq-bridge version      Print version`)
}

func fatal(err error) {
	slog.Error("mcqq-bridge failed", "error", err)
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
