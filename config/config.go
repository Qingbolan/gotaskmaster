package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogDir    string           `json:"log_dir" yaml:"log_dir"`
	ConfigDir string		   `json:"config_dir" yaml:"config_dir"`
	Processes []ProcessConfig  `json:"processes" yaml:"processes"`
}

type ProcessConfig struct {
    Name         string   `json:"name" yaml:"name"`
    Command      string   `json:"command" yaml:"command"`
    Args         []string `json:"args" yaml:"args"`
    Env          []string `json:"env" yaml:"env"`
    WorkDir      string   `json:"work_dir" yaml:"work_dir"`
    MaxInstances int      `json:"max_instances" yaml:"max_instances"`
    AutoStart    bool     `json:"auto_start" yaml:"auto_start"` // 新增
}

var (
	config *Config
	mu     sync.RWMutex
)

func LoadConfig(filename string) (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var cfg Config

	switch filepath.Ext(filename) {
	case ".json":
		if err := json.NewDecoder(file).Decode(&cfg); err != nil {
			return nil, fmt.Errorf("failed to decode JSON config: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.NewDecoder(file).Decode(&cfg); err != nil {
			return nil, fmt.Errorf("failed to decode YAML config: %w", err)
		}
	default:
		return nil, errors.New("unsupported config file format")
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	setDefaults(&cfg)

	config = &cfg
	return config, nil
}

func validateConfig(cfg *Config) error {
	if cfg.LogDir == "" {
		return errors.New("log_dir must be specified")
	}

	if len(cfg.Processes) == 0 {
		return errors.New("at least one process must be configured")
	}

	for i, proc := range cfg.Processes {
		if proc.Name == "" {
			return fmt.Errorf("process #%d: name must be specified", i+1)
		}
		if proc.Command == "" {
			return fmt.Errorf("process '%s': command must be specified", proc.Name)
		}
	}

	return nil
}

func setDefaults(cfg *Config) {
	cfg.LogDir = filepath.Clean(cfg.LogDir)

	for i := range cfg.Processes {
		if cfg.Processes[i].MaxInstances <= 0 {
			cfg.Processes[i].MaxInstances = 1
		}
		if cfg.Processes[i].WorkDir != "" {
			cfg.Processes[i].WorkDir = filepath.Clean(cfg.Processes[i].WorkDir)
		}
	}
}

func GetConfig() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return config
}

func SaveConfig(filename string) error {
	mu.RLock()
	defer mu.RUnlock()

	if config == nil {
		return errors.New("no configuration loaded")
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	switch filepath.Ext(filename) {
	case ".json":
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(config); err != nil {
			return fmt.Errorf("failed to encode JSON config: %w", err)
		}
	case ".yaml", ".yml":
		encoder := yaml.NewEncoder(file)
		encoder.SetIndent(2)
		if err := encoder.Encode(config); err != nil {
			return fmt.Errorf("failed to encode YAML config: %w", err)
		}
		encoder.Close()
	default:
		return errors.New("unsupported config file format")
	}

	return nil
}