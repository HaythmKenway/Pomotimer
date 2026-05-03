package main

import (
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	Steps []ConfigStep `json:"steps"`
}

type ConfigStep struct {
	Name              string `json:"name"`
	Duration          int    `json:"duration_minutes"`
	RequiresRecording bool   `json:"requires_recording"`
}

func loadConfig() ([]Step, error) {
	configPath := "config.json"
	var config Config

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		config = Config{
			Steps: []ConfigStep{
				{Name: "Focus", Duration: 25, RequiresRecording: true},
				{Name: "Short Break", Duration: 5, RequiresRecording: false},
				{Name: "Long Break", Duration: 15, RequiresRecording: false},
			},
		}
		data, _ := json.MarshalIndent(config, "", "  ")
		os.WriteFile(configPath, data, 0644)
	} else {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, err
		}
	}

	steps := make([]Step, len(config.Steps))
	for i, cs := range config.Steps {
		steps[i] = Step{
			Name:              cs.Name,
			Duration:          time.Duration(cs.Duration) * time.Minute,
			RequiresRecording: cs.RequiresRecording,
		}
	}

	return steps, nil
}
