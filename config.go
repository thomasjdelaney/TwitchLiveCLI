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
	Display              string
	SortBy               string
	SortOrder            string
	SortOrderSet         bool
	RefreshInterval      time.Duration
	Subscriptions        bool
	Viewer               string
	Player               string
	Chat                 bool
	Quality              string
	LowLatency           bool
	NotificationChannels []string
	NotifyOnStartup      bool
	ConfigPath           string
	ConfigExplicit       bool
}

type configFile struct {
	Display              string   `toml:"display"`
	SortBy               string   `toml:"sort_by"`
	SortOrder            string   `toml:"sort_order"`
	RefreshInterval      *string  `toml:"refresh_interval"`
	Subscriptions        *bool    `toml:"subscriptions"`
	Viewer               string   `toml:"viewer"`
	Player               string   `toml:"player"`
	Chat                 *bool    `toml:"chat"`
	Quality              string   `toml:"quality"`
	LowLatency           *bool    `toml:"low_latency"`
	NotificationChannels []string `toml:"notification_channels"`
	NotifyOnStartup      *bool    `toml:"notify_on_startup"`
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
	resolved, err := loadPreferencesFile(path, !explicitPath)
	if err != nil {
		return preferences{}, err
	}
	resolved.ConfigPath = path
	resolved.ConfigExplicit = explicitPath
	return resolved, nil
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
	resolved.NotificationChannels = uniqueNonEmpty(file.NotificationChannels)
	if file.NotifyOnStartup != nil {
		resolved.NotifyOnStartup = *file.NotifyOnStartup
	}
	if resolved.SortOrder == "" {
		resolved.SortOrder = defaultSortOrderFor(resolved.SortBy)
	}
	return resolved, nil
}

func savePreferences(p preferences) error {
	if p.ConfigPath == "" {
		return errors.New("config path is not set")
	}
	data, err := notificationPreferencesConfig(p)
	if err != nil {
		return err
	}
	if p.ConfigExplicit {
		if _, err := os.Stat(p.ConfigPath); err != nil {
			return fmt.Errorf("read config %s: %w", p.ConfigPath, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.ConfigPath), 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p.ConfigPath), ".config-*.toml")
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("set config permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(tmpName, p.ConfigPath); err != nil {
		return fmt.Errorf("save config %s: %w", p.ConfigPath, err)
	}
	return nil
}

// notificationPreferencesConfig updates only notification settings in the
// existing TOML source, retaining every unrelated byte including comments and
// whitespace. A missing default config is initialized with just these settings.
func notificationPreferencesConfig(p preferences) ([]byte, error) {
	original, err := os.ReadFile(p.ConfigPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read config %s: %w", p.ConfigPath, err)
	}
	if errors.Is(err, os.ErrNotExist) && p.ConfigExplicit {
		return nil, fmt.Errorf("read config %s: %w", p.ConfigPath, err)
	}
	channels := uniqueNonEmpty(p.NotificationChannels)
	encodedChannels, err := toml.Marshal(struct {
		Channels []string `toml:"notification_channels"`
	}{Channels: channels})
	if err != nil {
		return nil, fmt.Errorf("encode notification preferences: %w", err)
	}
	encodedStartup, err := toml.Marshal(struct {
		Startup bool `toml:"notify_on_startup"`
	}{Startup: p.NotifyOnStartup})
	if err != nil {
		return nil, fmt.Errorf("encode notification preferences: %w", err)
	}
	channelValue := strings.TrimSpace(strings.TrimPrefix(string(encodedChannels), "notification_channels ="))
	startupValue := strings.TrimSpace(strings.TrimPrefix(string(encodedStartup), "notify_on_startup ="))
	return replaceNotificationSettings(original, channelValue, startupValue), nil
}

func replaceNotificationSettings(source []byte, channels, startup string) []byte {
	newline := "\n"
	if bytes.Contains(source, []byte("\r\n")) {
		newline = "\r\n"
	}
	text := string(source)
	spans := notificationSettingSpans(text)
	replacements := []struct {
		key, value string
	}{
		{"notification_channels", channels},
		{"notify_on_startup", startup},
	}
	for i := len(spans) - 1; i >= 0; i-- {
		span := spans[i]
		value := replacements[span.setting].value
		if span.setting == 0 {
			value = preserveArrayComments(text[span.start:span.end], value, newline)
		}
		text = text[:span.start] + value + text[span.end:]
	}
	for setting := range replacements {
		found := false
		for _, span := range spans {
			if span.setting == setting {
				found = true
				break
			}
		}
		if !found {
			if text != "" && !strings.HasSuffix(text, "\n") {
				text += newline
			}
			text += replacements[setting].key + " = " + replacements[setting].value + newline
		}
	}
	return []byte(text)
}

func preserveArrayComments(original, replacement, newline string) string {
	if !strings.HasPrefix(strings.TrimSpace(original), "[") || !strings.HasSuffix(strings.TrimSpace(original), "]") {
		return replacement
	}
	var comments []string
	for _, line := range strings.Split(original, "\n") {
		line = strings.TrimSuffix(line, "\r")
		quote := byte(0)
		escaped := false
		for i := 0; i < len(line); i++ {
			c := line[i]
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				continue
			}
			if c == '"' || c == '\'' {
				quote = c
				continue
			}
			if c == '#' {
				indentEnd := 0
				for indentEnd < i && (line[indentEnd] == ' ' || line[indentEnd] == '\t') {
					indentEnd++
				}
				comments = append(comments, line[:indentEnd]+line[i:])
				break
			}
		}
	}
	if len(comments) == 0 {
		return replacement
	}
	trimmed := strings.TrimSpace(replacement)
	return strings.TrimSuffix(trimmed, "]") + newline + strings.Join(comments, newline) + newline + "]"
}

type notificationSettingSpan struct{ start, end, setting int }

// notificationSettingSpans finds top-level notification assignments and only
// returns the value ranges, leaving surrounding spacing and comments intact.
func notificationSettingSpans(source string) []notificationSettingSpan {
	var spans []notificationSettingSpan
	lines := strings.SplitAfter(source, "\n")
	offset, section := 0, false
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSuffix(strings.TrimSuffix(lines[i], "\n"), "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			section = true
			offset += len(lines[i])
			continue
		}
		if section || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			offset += len(lines[i])
			continue
		}
		setting, keyLen := -1, 0
		for index, key := range []string{"notification_channels", "notify_on_startup"} {
			if strings.HasPrefix(trimmed, key) {
				rest := strings.TrimSpace(strings.TrimPrefix(trimmed, key))
				if strings.HasPrefix(rest, "=") {
					setting, keyLen = index, len(key)
					break
				}
			}
		}
		if setting < 0 {
			offset += len(lines[i])
			continue
		}
		lineStart := offset + strings.Index(lines[i], trimmed)
		valueStart := lineStart + keyLen
		for valueStart < len(source) && (source[valueStart] == ' ' || source[valueStart] == '\t') {
			valueStart++
		}
		if valueStart >= len(source) || source[valueStart] != '=' {
			offset += len(lines[i])
			continue
		}
		valueStart++
		for valueStart < len(source) && (source[valueStart] == ' ' || source[valueStart] == '\t') {
			valueStart++
		}
		valueEnd := scanTOMLValueEnd(source, valueStart)
		spans = append(spans, notificationSettingSpan{start: valueStart, end: valueEnd, setting: setting})
		lineEnd := offset + len(lines[i])
		for lineEnd < valueEnd && i+1 < len(lines) {
			offset = lineEnd
			i++
			lineEnd = offset + len(lines[i])
		}
		offset = lineEnd
	}
	return spans
}

func scanTOMLValueEnd(source string, start int) int {
	depth, quote, escaped, comment := 0, byte(0), false, false
	for i := start; i < len(source); i++ {
		c := source[i]
		if comment {
			if c == '\n' {
				return i
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '#':
			if depth == 0 {
				for i > start && (source[i-1] == ' ' || source[i-1] == '\t') {
					i--
				}
				return i
			}
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		case '\n':
			if depth == 0 {
				for i > start && (source[i-1] == ' ' || source[i-1] == '\t' || source[i-1] == '\r') {
					i--
				}
				return i
			}
		}
	}
	return len(source)
}

func boolPointer(value bool) *bool { return &value }

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
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
