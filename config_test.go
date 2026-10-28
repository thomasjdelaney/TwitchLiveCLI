package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLoadPreferencesDefaultsWhenOptionalConfigIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")
	got, err := loadPreferencesFile(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Display != "table" || got.SortBy != "viewers" || got.SortOrder != "desc" || got.RefreshInterval != time.Minute || got.Subscriptions || got.Viewer != "browser" || got.Player != "mpv" || got.Chat || got.Quality != "middle" || got.LowLatency {
		t.Fatalf("preferences = %+v", got)
	}
	if got.NotifyOnStartup || len(got.NotificationChannels) != 0 {
		t.Fatalf("notification defaults = %+v", got)
	}
}

func TestNotificationPreferencesSaveAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	prefs := defaultPreferences()
	prefs.ConfigPath = path
	prefs.NotificationChannels = []string{"id-1", "id-2", "id-1", ""}
	prefs.NotifyOnStartup = true
	if err := savePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	got, err := loadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.NotificationChannels, ",") != "id-1,id-2" || !got.NotifyOnStartup {
		t.Fatalf("notification preferences = %+v", got)
	}
	if got.Viewer != prefs.Viewer || got.RefreshInterval != prefs.RefreshInterval {
		t.Fatalf("unrelated preferences changed: %+v", got)
	}
	prefs.NotifyOnStartup = false
	if err := savePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	got, err = loadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.NotifyOnStartup {
		t.Fatalf("startup alerts should round-trip false: %+v", got)
	}
}

func TestNotificationPreferencesSaveToDefaultConfigPath(t *testing.T) {
	root := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", root)
	case "darwin":
		t.Setenv("HOME", root)
	default:
		t.Setenv("XDG_CONFIG_HOME", root)
	}
	prefs, err := loadPreferences("")
	if err != nil {
		t.Fatal(err)
	}
	prefs.NotificationChannels = []string{"channel-id"}
	if err := savePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	got, err := loadPreferences("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.NotificationChannels) != 1 || got.NotificationChannels[0] != "channel-id" {
		t.Fatalf("default-path preferences = %+v", got)
	}
}

func TestSaveNotificationPreferencesPreservesConfigSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "# keep this comment\r\ndisplay   = 'card' # keep inline comment\r\nnotification_channels = [\r\n  \"old-id\", # keep array comment\r\n]\r\nnotify_on_startup = false   # keep startup note\r\n\r\n# trailing comment"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	prefs, err := loadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	prefs.NotificationChannels = []string{"new-id", "two"}
	prefs.NotifyOnStartup = true
	if err := savePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	want := "# keep this comment\r\ndisplay   = 'card' # keep inline comment\r\nnotification_channels = ['new-id', 'two'\r\n  # keep array comment\r\n]\r\nnotify_on_startup = true   # keep startup note\r\n\r\n# trailing comment"
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("saved config bytes differ\n got: %q\nwant: %q", got, want)
	}
	if _, err := loadPreferences(path); err != nil {
		t.Fatalf("saved config is not valid TOML: %v", err)
	}
}

func TestSaveNotificationPreferencesAppendsMissingKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("# existing\nviewer = 'browser'"), 0600); err != nil {
		t.Fatal(err)
	}
	prefs, err := loadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := savePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "# existing\nviewer = 'browser'\nnotification_channels = []\nnotify_on_startup = false\n"
	if string(got) != want {
		t.Fatalf("saved config = %q, want %q", got, want)
	}
}

func TestSaveExplicitConfigRequiresExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("# config\n"), 0600); err != nil {
		t.Fatal(err)
	}
	prefs, err := loadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := savePreferences(prefs); err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("savePreferences error = %v, want missing explicit config error", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing explicit config was recreated: %v", err)
	}
}

func TestLoadPreferencesViewerSettings(t *testing.T) {
	got := writeAndLoadPreferences(t, "viewer = 'streamlink'\nplayer = 'vlc'\nchat = true\nquality = '720p60'\nlow_latency = true\n")
	if got.Viewer != "streamlink" || got.Player != "vlc" || !got.Chat || got.Quality != "720p60" || !got.LowLatency {
		t.Fatalf("viewer preferences = %+v", got)
	}
}

func TestLoadPreferencesSubscriptionSetting(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{name: "enabled", text: "subscriptions = true\n", want: true},
		{name: "disabled", text: "subscriptions = false\n", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := writeAndLoadPreferences(t, tc.text)
			if got.Subscriptions != tc.want {
				t.Fatalf("subscriptions = %v, want %v", got.Subscriptions, tc.want)
			}
		})
	}
}

func TestLoadPreferencesPartialConfig(t *testing.T) {
	got := writeAndLoadPreferences(t, "display = 'card'\nsort_by = 'channel'\nrefresh_interval = '45s'\n")
	if got.Display != "card" || got.SortBy != "channel" || got.SortOrder != "asc" || got.RefreshInterval != 45*time.Second {
		t.Fatalf("preferences = %+v", got)
	}
}

func TestLoadPreferencesRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{name: "malformed TOML", text: "display = [", want: "parse config"},
		{name: "unknown key", text: "dispay = 'card'", want: "parse config"},
		{name: "display", text: "display = 'cards'", want: "display must be"},
		{name: "sort key", text: "sort_by = 'followers'", want: "invalid sort_by"},
		{name: "sort order", text: "sort_order = 'sideways'", want: "sort_order must be"},
		{name: "duration", text: "refresh_interval = 'later'", want: "refresh_interval: invalid duration"},
		{name: "zero duration", text: "refresh_interval = '0s'", want: "refresh_interval: must be at least 2s"},
		{name: "below minimum", text: "refresh_interval = '1999ms'", want: "refresh_interval: must be at least 2s"},
		{name: "empty duration", text: "refresh_interval = ''", want: "refresh_interval: invalid duration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.text), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := loadPreferences(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadPreferences error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestLoadPreferencesAcceptsRefreshIntervalMinimum(t *testing.T) {
	for _, value := range []string{"2s", "2500ms", "3s"} {
		t.Run(value, func(t *testing.T) {
			got := writeAndLoadPreferences(t, "refresh_interval = '"+value+"'\n")
			if got.RefreshInterval < minimumRefreshInterval {
				t.Fatalf("refresh interval = %s, want at least %s", got.RefreshInterval, minimumRefreshInterval)
			}
		})
	}
}

func TestLoadPreferencesRequiresExplicitConfig(t *testing.T) {
	_, err := loadPreferences(filepath.Join(t.TempDir(), "missing.toml"))
	if err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("loadPreferences error = %v, want missing-file error", err)
	}
}

func TestResolvePreferencesCLIOverridesConfig(t *testing.T) {
	base := preferences{
		Display:         "card",
		SortBy:          "viewers",
		SortOrder:       "desc",
		RefreshInterval: time.Minute,
	}
	got, err := resolvePreferences(base, preferenceOverrides{
		Display:         override[string]{Value: "table", Set: true},
		SortBy:          override[string]{Value: "channel", Set: true},
		RefreshInterval: override[string]{Value: "30s", Set: true},
		Subscriptions:   override[bool]{Value: true, Set: true},
		Viewer:          override[string]{Value: "streamlink", Set: true},
		Player:          override[string]{Value: "vlc", Set: true},
		Chat:            override[bool]{Value: true, Set: true},
		Quality:         override[string]{Value: "720p60", Set: true},
		LowLatency:      override[bool]{Value: true, Set: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Display != "table" || got.SortBy != "channel" || got.SortOrder != "asc" || got.RefreshInterval != 30*time.Second || !got.Subscriptions || got.Viewer != "streamlink" || got.Player != "vlc" || !got.Chat || got.Quality != "720p60" || !got.LowLatency {
		t.Fatalf("resolved preferences = %+v", got)
	}
}

func TestResolvePreferencesPreservesConfiguredSortOrder(t *testing.T) {
	base := preferences{Display: "table", SortBy: "viewers", SortOrder: "desc", SortOrderSet: true, RefreshInterval: time.Minute}
	got, err := resolvePreferences(base, preferenceOverrides{SortBy: override[string]{Value: "channel", Set: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got.SortBy != "channel" || got.SortOrder != "desc" {
		t.Fatalf("resolved preferences = %+v", got)
	}
}

func TestResolvePreferencesRejectsInvalidCLIValues(t *testing.T) {
	base := preferences{Display: "table", SortBy: "viewers", SortOrder: "desc", RefreshInterval: time.Minute}
	for _, tc := range []struct {
		name      string
		overrides preferenceOverrides
		want      string
	}{
		{name: "display", overrides: preferenceOverrides{Display: override[string]{Value: "cards", Set: true}}, want: "--display"},
		{name: "sort key", overrides: preferenceOverrides{SortBy: override[string]{Value: "followers", Set: true}}, want: "--sort-by"},
		{name: "sort order", overrides: preferenceOverrides{SortOrder: override[string]{Value: "sideways", Set: true}}, want: "--sort-order"},
		{name: "refresh interval", overrides: preferenceOverrides{RefreshInterval: override[string]{Value: "0s", Set: true}}, want: "--refresh-interval"},
		{name: "below minimum", overrides: preferenceOverrides{RefreshInterval: override[string]{Value: "1999ms", Set: true}}, want: "at least 2s"},
		{name: "subscription sort requires opt-in", overrides: preferenceOverrides{SortBy: override[string]{Value: "subscribed", Set: true}}, want: "requires --subscriptions"},
		{name: "viewer", overrides: preferenceOverrides{Viewer: override[string]{Value: "vlc", Set: true}}, want: "viewer must"},
		{name: "player", overrides: preferenceOverrides{Player: override[string]{Value: "ffplay", Set: true}}, want: "player must"},
		{name: "quality whitespace", overrides: preferenceOverrides{Quality: override[string]{Value: "720p 60", Set: true}}, want: "quality must"},
		{name: "low latency requires Streamlink", overrides: preferenceOverrides{LowLatency: override[bool]{Value: true, Set: true}}, want: "low_latency requires"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolvePreferences(base, tc.overrides)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("resolvePreferences error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestResolvePreferencesAllowsDisablingConfiguredChatAndLatency(t *testing.T) {
	base := preferences{Display: "table", SortBy: "viewers", SortOrder: "desc", RefreshInterval: time.Minute, Viewer: "streamlink", Player: "mpv", Quality: "middle", Chat: true, LowLatency: true}
	got, err := resolvePreferences(base, preferenceOverrides{
		Viewer:     override[string]{Value: "streamlink", Set: true},
		Chat:       override[bool]{Value: false, Set: true},
		LowLatency: override[bool]{Value: false, Set: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Chat || got.LowLatency {
		t.Fatalf("explicit false values were not applied: %+v", got)
	}
}

func TestResolvePreferencesSubscriptionOverrideAndSortRequirement(t *testing.T) {
	base := preferences{Display: "table", SortBy: "viewers", SortOrder: "desc", RefreshInterval: time.Minute, Subscriptions: true}
	got, err := resolvePreferences(base, preferenceOverrides{Subscriptions: override[bool]{Value: false, Set: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Subscriptions {
		t.Fatal("explicit false should override enabled config")
	}
	_, err = resolvePreferences(preferences{Display: "table", SortBy: "sub_tier", SortOrder: "desc", RefreshInterval: time.Minute}, preferenceOverrides{})
	if err == nil || !strings.Contains(err.Error(), "requires --subscriptions") {
		t.Fatalf("subscription sort error = %v", err)
	}
}

func TestResolvePreferencesConfigSubscriptionSort(t *testing.T) {
	base := writeAndLoadPreferences(t, "sort_by = 'sub_tier'\n")
	if _, err := resolvePreferences(base, preferenceOverrides{}); err == nil || !strings.Contains(err.Error(), "requires --subscriptions") {
		t.Fatalf("subscription sort error = %v", err)
	}
	base = writeAndLoadPreferences(t, "subscriptions = true\nsort_by = 'sub_tier'\n")
	if _, err := resolvePreferences(base, preferenceOverrides{}); err != nil {
		t.Fatalf("enabled subscription sort returned error: %v", err)
	}
}

func TestResolvePreferencesAcceptsRefreshIntervalMinimum(t *testing.T) {
	base := preferences{Display: "table", SortBy: "viewers", SortOrder: "desc", RefreshInterval: time.Minute}
	got, err := resolvePreferences(base, preferenceOverrides{RefreshInterval: override[string]{Value: "2s", Set: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got.RefreshInterval != minimumRefreshInterval {
		t.Fatalf("refresh interval = %s, want %s", got.RefreshInterval, minimumRefreshInterval)
	}
}

func writeAndLoadPreferences(t *testing.T, contents string) preferences {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := loadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
