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

func main() {
	configPath := flag.String("config", "config.json", "Path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		log.Fatalf("Error creating log directory: %v", err)
	}

	logFile, err := os.OpenFile(filepath.Join(cfg.LogDir, "gotaskmaster.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Error opening log file: %v", err)
	}
	defer logFile.Close()

	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	pm := process.NewManager(cfg)
	go pm.Start()

	if err := ui.Run(pm); err != nil {
		fmt.Printf("Error running UI: %v\n", err)
		os.Exit(1)
	}
}