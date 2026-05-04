package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type persistedConfig struct {
	TmuxMode       string `json:"tmux_mode,omitempty"`
	PopupWidth     string `json:"popup_width,omitempty"`
	PopupHeight    string `json:"popup_height,omitempty"`
	PopupX         string `json:"popup_x,omitempty"`
	PopupY         string `json:"popup_y,omitempty"`
	SplitDirection string `json:"split_direction,omitempty"`
	SplitSize      string `json:"split_size,omitempty"`
}

func defaultPersistedConfig() persistedConfig {
	return persistedConfig{
		TmuxMode:       "split",
		PopupWidth:     "90%",
		PopupHeight:    "90%",
		PopupX:         "",
		PopupY:         "",
		SplitDirection: "right",
		SplitSize:      "50%",
	}
}

func loadPersistedConfig() (persistedConfig, bool) {
	path, err := configFilePath()
	if err != nil {
		return persistedConfig{}, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return persistedConfig{}, false
	}

	var cfg persistedConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return persistedConfig{}, false
	}

	return cfg, true
}

func savePersistedConfig(cfg persistedConfig) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func resetPersistedConfig() error {
	return savePersistedConfig(defaultPersistedConfig())
}

func configFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gitws", "config.json"), nil
}
