package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func resolveDataPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if path := os.Getenv("TASKCHUTE_DATA"); path != "" {
		return path, nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "taskchute-for-terminal")
	filename := filepath.Join(dir, "config.json")
	raw, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return filepath.Join(dir, "tasks.json"), nil
	}
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}
	var config struct {
		DataPath string `json:"data_path"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", fmt.Errorf("parse %s: %w", filename, err)
	}
	if filepath.IsAbs(config.DataPath) {
		return config.DataPath, nil
	}
	if config.DataPath == "" {
		return filepath.Join(dir, "tasks.json"), nil
	}
	return filepath.Join(dir, config.DataPath), nil
}
