package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	Subscriptions   bool
	Viewer          string
	Player          string
	Chat            bool
	Quality         string
	LowLatency      bool
}

type configFile struct {
	Display         string  `toml:"display"`
	SortBy          string  `toml:"sort_by"`
	SortOrder       string  `toml:"sort_order"`
	RefreshInterval *string `toml:"refresh_interval"`
	Subscriptions   *bool   `toml:"subscriptions"`
	Viewer          string  `toml:"viewer"`
	Player          string  `toml:"player"`
	Chat            *bool   `toml:"chat"`
	Quality         string  `toml:"quality"`
	LowLatency      *bool   `toml:"low_latency"`
}

func defaultPreferences() preferences {
	return preferences{
		Display:         defaultDisplay,
		SortBy:          defaultSortBy,
		RefreshInterval: defaultConfigInterval,
		Viewer:          defaultViewer,
		Player:          defaultPlayer,
		Quality:         defaultQuality,
	}
}

func viewerOptionsFromPreferences(p preferences) viewerOptions {
	return viewerOptions{Viewer: p.Viewer, Player: p.Player, Chat: p.Chat, Quality: p.Quality, LowLatency: p.LowLatency}
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
	if file.Subscriptions != nil {
		resolved.Subscriptions = *file.Subscriptions
	}
	if file.Viewer != "" {
		resolved.Viewer = file.Viewer
	}
	if file.Player != "" {
		resolved.Player = file.Player
	}
	if file.Chat != nil {
		resolved.Chat = *file.Chat
	}
	if file.Quality != "" {
		resolved.Quality = file.Quality
	}
	if file.LowLatency != nil {
		resolved.LowLatency = *file.LowLatency
	}
	if resolved.SortOrder == "" {
		resolved.SortOrder = defaultSortOrderFor(resolved.SortBy)
	}
	return resolved, nil
}

type override[T any] struct {
	Value T
	Set   bool
}

type preferenceOverrides struct {
	Display         override[string]
	SortBy          override[string]
	SortOrder       override[string]
	RefreshInterval override[string]
	Subscriptions   override[bool]
	Viewer          override[string]
	Player          override[string]
	Chat            override[bool]
	Quality         override[string]
	LowLatency      override[bool]
}

func resolvePreferences(base preferences, overrides preferenceOverrides) (preferences, error) {
	resolved := base
	if resolved.Viewer == "" {
		resolved.Viewer = defaultViewer
	}
	if resolved.Player == "" {
		resolved.Player = defaultPlayer
	}
	if resolved.Quality == "" {
		resolved.Quality = defaultQuality
	}
	if overrides.Subscriptions.Set {
		resolved.Subscriptions = overrides.Subscriptions.Value
	}
	if overrides.Viewer.Set {
		resolved.Viewer = overrides.Viewer.Value
	}
	if overrides.Player.Set {
		resolved.Player = overrides.Player.Value
	}
	if overrides.Chat.Set {
		resolved.Chat = overrides.Chat.Value
	}
	if overrides.Quality.Set {
		resolved.Quality = overrides.Quality.Value
	}
	if overrides.LowLatency.Set {
		resolved.LowLatency = overrides.LowLatency.Value
	}
	if overrides.Display.Set {
		if overrides.Display.Value != "table" && overrides.Display.Value != "card" {
			return preferences{}, fmt.Errorf("--display must be table or card, got %q", overrides.Display.Value)
		}
		resolved.Display = overrides.Display.Value
	}
	if overrides.SortBy.Set {
		if !validSortBy(overrides.SortBy.Value) {
			return preferences{}, fmt.Errorf("invalid --sort-by %q", overrides.SortBy.Value)
		}
		resolved.SortBy = overrides.SortBy.Value
	}
	if overrides.SortOrder.Set {
		if overrides.SortOrder.Value != "asc" && overrides.SortOrder.Value != "desc" {
			return preferences{}, fmt.Errorf("--sort-order must be asc or desc, got %q", overrides.SortOrder.Value)
		}
		resolved.SortOrder = overrides.SortOrder.Value
		resolved.SortOrderSet = true
	} else if !resolved.SortOrderSet && overrides.SortBy.Set {
		resolved.SortOrder = defaultSortOrderFor(resolved.SortBy)
	}
	if overrides.RefreshInterval.Set {
		interval, err := parseRefreshInterval(overrides.RefreshInterval.Value)
		if err != nil {
			return preferences{}, fmt.Errorf("--refresh-interval: %w", err)
		}
		resolved.RefreshInterval = interval
	}
	if requiresSubscriptions(resolved.SortBy) && !resolved.Subscriptions {
		return preferences{}, fmt.Errorf("sort_by %q requires --subscriptions or subscriptions = true", resolved.SortBy)
	}
	if resolved.Viewer != "browser" && resolved.Viewer != "streamlink" {
		return preferences{}, fmt.Errorf("viewer must be browser or streamlink, got %q", resolved.Viewer)
	}
	if resolved.Player != "mpv" && resolved.Player != "vlc" {
		return preferences{}, fmt.Errorf("player must be mpv or vlc, got %q", resolved.Player)
	}
	if resolved.Quality == "" || strings.ContainsAny(resolved.Quality, " \t\r\n") {
		return preferences{}, fmt.Errorf("quality must be a non-empty Streamlink quality name without whitespace")
	}
	if resolved.LowLatency && resolved.Viewer != "streamlink" {
		return preferences{}, fmt.Errorf("low_latency requires viewer = \"streamlink\"")
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
	case "viewers", "channel", "title", "category", "live_duration", "subscribed", "sub_tier":
		return true
	default:
		return false
	}
}

func requiresSubscriptions(sortBy string) bool {
	return sortBy == "subscribed" || sortBy == "sub_tier"
}

func defaultSortOrderFor(sortBy string) string {
	if sortBy == "channel" || sortBy == "title" || sortBy == "category" {
		return "asc"
	}
	return "desc"
}
