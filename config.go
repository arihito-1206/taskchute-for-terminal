package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type settings struct {
	DataPath     string    `json:"data_path"`
	Sections     []section `json:"sections"`
	DayEndTarget string    `json:"day_end_target"`
}

func loadSettings(explicit string) (settings, error) {
	path := explicit
	if path == "" {
		path = os.Getenv("TASKCHUTE_DATA")
	}
	root, err := os.UserConfigDir()
	if err != nil {
		if path != "" {
			config := settings{DataPath: path}
			return config, config.prepareSections()
		}
		return settings{}, err
	}
	dir := filepath.Join(root, "taskchute-for-terminal")
	filename := filepath.Join(dir, "config.json")
	var config settings
	raw, err := os.ReadFile(filename)
	if err == nil {
		if err := json.Unmarshal(raw, &config); err != nil {
			return settings{}, fmt.Errorf("parse %s: %w", filename, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return settings{}, fmt.Errorf("read config: %w", err)
	}
	if config.DayEndTarget != "" && config.DayEndTarget != "24:00" && !validSectionTime(config.DayEndTarget) {
		return settings{}, fmt.Errorf("parse %s: day_end_target must use HH:MM (24:00 allowed)", filename)
	}
	if err := config.prepareSections(); err != nil {
		return settings{}, fmt.Errorf("parse %s: %w", filename, err)
	}
	switch {
	case path != "":
		config.DataPath = path
	case config.DataPath == "":
		config.DataPath = filepath.Join(dir, "tasks.json")
	case !filepath.IsAbs(config.DataPath):
		config.DataPath = filepath.Join(dir, config.DataPath)
	}
	return config, nil
}

func (c *settings) prepareSections() error {
	if c.Sections == nil {
		c.Sections = []section{
			{ID: "overnight", Name: "Overnight", Start: "00:00", End: "06:00"},
			{ID: "morning", Name: "Morning", Start: "06:00", End: "12:00"},
			{ID: "afternoon", Name: "Afternoon", Start: "12:00", End: "18:00"},
			{ID: "evening", Name: "Evening", Start: "18:00", End: "24:00"},
		}
	}
	return validateSections(c.Sections)
}
