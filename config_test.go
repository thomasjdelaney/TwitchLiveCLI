package main

import (
	"os"
	"path/filepath"
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
	if got.Display != "table" || got.SortBy != "viewers" || got.SortOrder != "desc" || got.RefreshInterval != time.Minute || got.Subscriptions {
		t.Fatalf("preferences = %+v", got)
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
		Display:          "table",
		SortBy:           "channel",
		RefreshInterval:  "30s",
		Subscriptions:    true,
		DisplaySet:       true,
		SortBySet:        true,
		RefreshSet:       true,
		SubscriptionsSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Display != "table" || got.SortBy != "channel" || got.SortOrder != "asc" || got.RefreshInterval != 30*time.Second || !got.Subscriptions {
		t.Fatalf("resolved preferences = %+v", got)
	}
}

func TestResolvePreferencesPreservesConfiguredSortOrder(t *testing.T) {
	base := preferences{Display: "table", SortBy: "viewers", SortOrder: "desc", SortOrderSet: true, RefreshInterval: time.Minute}
	got, err := resolvePreferences(base, preferenceOverrides{SortBy: "channel", SortBySet: true})
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
		{name: "display", overrides: preferenceOverrides{Display: "cards", DisplaySet: true}, want: "--display"},
		{name: "sort key", overrides: preferenceOverrides{SortBy: "followers", SortBySet: true}, want: "--sort-by"},
		{name: "sort order", overrides: preferenceOverrides{SortOrder: "sideways", SortOrderSet: true}, want: "--sort-order"},
		{name: "refresh interval", overrides: preferenceOverrides{RefreshInterval: "0s", RefreshSet: true}, want: "--refresh-interval"},
		{name: "below minimum", overrides: preferenceOverrides{RefreshInterval: "1999ms", RefreshSet: true}, want: "at least 2s"},
		{name: "subscription sort requires opt-in", overrides: preferenceOverrides{SortBy: "subscribed", SortBySet: true}, want: "requires --subscriptions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolvePreferences(base, tc.overrides)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("resolvePreferences error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestResolvePreferencesSubscriptionOverrideAndSortRequirement(t *testing.T) {
	base := preferences{Display: "table", SortBy: "viewers", SortOrder: "desc", RefreshInterval: time.Minute, Subscriptions: true}
	got, err := resolvePreferences(base, preferenceOverrides{SubscriptionsSet: true, Subscriptions: false})
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
	got, err := resolvePreferences(base, preferenceOverrides{RefreshInterval: "2s", RefreshSet: true})
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
