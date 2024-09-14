package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/qingbolan/gotaskmaster/config"
	"github.com/qingbolan/gotaskmaster/process"
	"github.com/qingbolan/gotaskmaster/ui"
)

var (
	configFile string
	logFile    string
)

func init() {
	flag.StringVar(&configFile, "config", "config.yaml", "Path to configuration file")
	flag.StringVar(&logFile, "log", "gotaskmaster.log", "Path to log file")
	flag.Parse()
}

func setupLogging() (*os.File, error) {
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	log.SetOutput(file)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	return file, nil
}

func main() {
	logFile, err := setupLogging()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up logging: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	log.Println("Starting GoTaskMaster")

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}
	log.Printf("Configuration loaded from %s", configFile)

	manager := process.NewManager(cfg)
	log.Println("Process Manager initialized")

	tui := ui.NewTUI(manager)
    if err := tui.Run(); err != nil {
        log.Fatalf("Error running TUI: %v", err)
    }

	// log.Println("GoTaskMaster shutting down")
}