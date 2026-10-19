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
	if got.Display != "table" || got.SortBy != "viewers" || got.SortOrder != "desc" || got.RefreshInterval != time.Minute {
		t.Fatalf("preferences = %+v", got)
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
		{name: "duration", text: "refresh_interval = 'later'", want: "refresh_interval must be"},
		{name: "nonpositive duration", text: "refresh_interval = '0s'", want: "refresh_interval must be"},
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
		Display:         "table",
		SortBy:          "channel",
		RefreshInterval: "30s",
		DisplaySet:      true,
		SortBySet:       true,
		RefreshSet:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Display != "table" || got.SortBy != "channel" || got.SortOrder != "asc" || got.RefreshInterval != 30*time.Second {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolvePreferences(base, tc.overrides)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("resolvePreferences error = %v, want substring %q", err, tc.want)
			}
		})
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
