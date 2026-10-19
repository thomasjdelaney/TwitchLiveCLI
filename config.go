package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	defaultDisplay         = "table"
	defaultSortBy          = "viewers"
	defaultSortOrder       = "desc"
	defaultConfigInterval  = time.Minute
	minimumRefreshInterval = 2 * time.Second
)

type preferences struct {
	Display         string
	SortBy          string
	SortOrder       string
	SortOrderSet    bool
	RefreshInterval time.Duration
}

type configFile struct {
	Display         string `toml:"display"`
	SortBy          string `toml:"sort_by"`
	SortOrder       string `toml:"sort_order"`
	RefreshInterval *string `toml:"refresh_interval"`
}

func defaultPreferences() preferences {
	return preferences{
		Display:         defaultDisplay,
		SortBy:          defaultSortBy,
		RefreshInterval: defaultConfigInterval,
	}
}

func defaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(configDir, "twitch-live-cli", "config.toml"), nil
}

func loadPreferences(path string) (preferences, error) {
	explicitPath := path != ""
	if !explicitPath {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return preferences{}, err
		}
	}
	return loadPreferencesFile(path, !explicitPath)
}

func loadPreferencesFile(path string, missingIsDefault bool) (preferences, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if missingIsDefault && errors.Is(err, os.ErrNotExist) {
			resolved := defaultPreferences()
			resolved.SortOrder = defaultSortOrderFor(resolved.SortBy)
			return resolved, nil
		}
		return preferences{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var file configFile
	decoder := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return preferences{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	resolved := defaultPreferences()
	if file.Display != "" {
		if file.Display != "table" && file.Display != "card" {
			return preferences{}, fmt.Errorf("config %s: display must be table or card, got %q", path, file.Display)
		}
		resolved.Display = file.Display
	}
	if file.SortBy != "" {
		if !validSortBy(file.SortBy) {
			return preferences{}, fmt.Errorf("config %s: invalid sort_by %q", path, file.SortBy)
		}
		resolved.SortBy = file.SortBy
	}
	if file.SortOrder != "" {
		if file.SortOrder != "asc" && file.SortOrder != "desc" {
			return preferences{}, fmt.Errorf("config %s: sort_order must be asc or desc, got %q", path, file.SortOrder)
		}
		resolved.SortOrder = file.SortOrder
		resolved.SortOrderSet = true
	}
	if file.RefreshInterval != nil {
		interval, err := parseRefreshInterval(*file.RefreshInterval)
		if err != nil {
			return preferences{}, fmt.Errorf("config %s: refresh_interval: %w", path, err)
		}
		resolved.RefreshInterval = interval
	}
	if resolved.SortOrder == "" {
		resolved.SortOrder = defaultSortOrderFor(resolved.SortBy)
	}
	return resolved, nil
}

type preferenceOverrides struct {
	Display         string
	SortBy          string
	SortOrder       string
	RefreshInterval string
	DisplaySet      bool
	SortBySet       bool
	SortOrderSet    bool
	RefreshSet      bool
}

func resolvePreferences(base preferences, overrides preferenceOverrides) (preferences, error) {
	resolved := base
	if overrides.DisplaySet {
		if overrides.Display != "table" && overrides.Display != "card" {
			return preferences{}, fmt.Errorf("--display must be table or card, got %q", overrides.Display)
		}
		resolved.Display = overrides.Display
	}
	if overrides.SortBySet {
		if !validSortBy(overrides.SortBy) {
			return preferences{}, fmt.Errorf("invalid --sort-by %q", overrides.SortBy)
		}
		resolved.SortBy = overrides.SortBy
	}
	if overrides.SortOrderSet {
		if overrides.SortOrder != "asc" && overrides.SortOrder != "desc" {
			return preferences{}, fmt.Errorf("--sort-order must be asc or desc, got %q", overrides.SortOrder)
		}
		resolved.SortOrder = overrides.SortOrder
		resolved.SortOrderSet = true
	} else if !resolved.SortOrderSet && overrides.SortBySet {
		resolved.SortOrder = defaultSortOrderFor(resolved.SortBy)
	}
	if overrides.RefreshSet {
		interval, err := parseRefreshInterval(overrides.RefreshInterval)
		if err != nil {
			return preferences{}, fmt.Errorf("--refresh-interval: %w", err)
		}
		resolved.RefreshInterval = interval
	}
	return resolved, nil
}

func parseRefreshInterval(value string) (time.Duration, error) {
	interval, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", value, err)
	}
	if interval < minimumRefreshInterval {
		return 0, fmt.Errorf("must be at least %s, got %q", minimumRefreshInterval, value)
	}
	return interval, nil
}

func validSortBy(value string) bool {
	switch value {
	case "viewers", "channel", "title", "category", "live_duration":
		return true
	default:
		return false
	}
}

func defaultSortOrderFor(sortBy string) string {
	if sortBy == "channel" || sortBy == "title" || sortBy == "category" {
		return "asc"
	}
	return "desc"
}
