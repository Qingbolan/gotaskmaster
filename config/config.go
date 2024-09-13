package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	LogDir string `json:"log_dir"`
	Processes []ProcessConfig `json:"processes"`
}

type ProcessConfig struct {
	Name         string   `json:"name"`
	Command      string   `json:"command"`
	Args         []string `json:"args"`
	Env          []string `json:"env"`
	MaxInstances int      `json:"max_instances"`
}

func Load(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}