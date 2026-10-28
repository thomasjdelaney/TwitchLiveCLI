package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestNotificationManagerTogglesAndFiltersChannels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	prefs := defaultPreferences()
	prefs.ConfigPath = path
	model := newNotificationManagerModel(context.Background(), nil, prefs, 80, 20, colorPalette{})
	model.followed = []Followed{{ID: "a", Name: "Alpha", Login: "alpha"}, {ID: "b", Name: "Beta", Login: "beta"}}
	model, _ = pressNotificationManagerKey(model, "/")
	model, _ = pressNotificationManagerKey(model, "b")
	if len(model.filteredChannels()) != 1 || model.filteredChannels()[0].ID != "b" {
		t.Fatalf("filtered channels = %+v", model.filteredChannels())
	}
	model, _ = pressNotificationManagerKey(model, "enter")
	model, _ = pressNotificationManagerKey(model, "space")
	if len(model.prefs.NotificationChannels) != 1 || model.prefs.NotificationChannels[0] != "b" {
		t.Fatalf("selected channels = %v", model.prefs.NotificationChannels)
	}
	loaded, err := loadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.NotificationChannels) != 1 || loaded.NotificationChannels[0] != "b" {
		t.Fatalf("saved channels = %v", loaded.NotificationChannels)
	}
}

func TestNotificationManagerAlphabeticalOrderAndNavigation(t *testing.T) {
	model := newNotificationManagerModel(context.Background(), nil, defaultPreferences(), 80, 20, colorPalette{})
	updated, _, _ := model.Update(followedChannelsMsg{channels: []Followed{{ID: "z", Name: "Zeta"}, {ID: "b", Name: "beta"}, {ID: "a", Name: "Alpha"}}})
	model = updated
	if got := []string{model.followed[0].Name, model.followed[1].Name, model.followed[2].Name}; strings.Join(got, ",") != "Alpha,beta,Zeta" {
		t.Fatalf("follow order = %v", got)
	}
	model.selected = 1
	model, _ = pressNotificationManagerKey(model, "j")
	if model.selected != 2 {
		t.Fatalf("j selected row %d, want down to 2", model.selected)
	}
	model, _ = pressNotificationManagerKey(model, "k")
	if model.selected != 1 {
		t.Fatalf("k selected row %d, want up to 1", model.selected)
	}
	model, _ = pressNotificationManagerKey(model, "down")
	if model.selected != 2 {
		t.Fatalf("down arrow selected row %d, want 2", model.selected)
	}
	model, _ = pressNotificationManagerKey(model, "up")
	if model.selected != 1 {
		t.Fatalf("up arrow selected row %d, want 1", model.selected)
	}
}

func TestNotificationManagerHighlightsStartupAndAnimatesLoading(t *testing.T) {
	prefs := defaultPreferences()
	prefs.NotifyOnStartup = true
	model := newNotificationManagerModel(context.Background(), nil, prefs, 80, 20, colorPalette{})
	model.loading = true
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"Startup alerts: ON", "press / to edit filter", "Loading followed channels"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("view missing %q:\n%s", want, plain)
		}
	}
	if got := ansi.Strip(model.spinner.View()); got != "" && !strings.Contains(plain, got) {
		t.Fatalf("loading view does not show spinner %q:\n%s", got, plain)
	}
	if strings.Index(plain, "DESKTOP NOTIFICATIONS") > strings.Index(plain, "Startup alerts:") || !strings.Contains(plain, strings.Repeat("─", 80)) {
		t.Fatalf("notification title and divider layout is incorrect:\n%s", plain)
	}
}

func TestNotificationManagerKeepsHelpVisibleWithLargeList(t *testing.T) {
	model := newNotificationManagerModel(context.Background(), nil, defaultPreferences(), 80, 24, colorPalette{})
	model.followed = make([]Followed, 100)
	for i := range model.followed {
		model.followed[i] = Followed{ID: fmt.Sprintf("%d", i), Name: fmt.Sprintf("Channel %d", i)}
	}
	plain := ansi.Strip(model.View().Content)
	lines := strings.Split(plain, "\n")
	if len(lines) > model.height {
		t.Fatalf("notification view has %d lines for %d-row terminal:\n%s", len(lines), model.height, plain)
	}
	if !strings.Contains(lines[len(lines)-1], "q quit") {
		t.Fatalf("help is not visible at the bottom:\n%s", plain)
	}
}

func TestNotificationFilterEscapeStopsEditingButKeepsFilter(t *testing.T) {
	model := newNotificationManagerModel(context.Background(), nil, defaultPreferences(), 80, 24, colorPalette{})
	model.filtering = true
	model.filter = "kimmy"
	model.followed = []Followed{{ID: "1", Name: "KimmyDNC"}}
	model, closed := pressNotificationManagerKey(model, "esc")
	if closed || model.filtering || model.filter != "kimmy" {
		t.Fatalf("filter state after escape = editing:%v filter:%q closed:%v", model.filtering, model.filter, closed)
	}
	plain := ansi.Strip(model.View().Content)
	if strings.Contains(plain, "Esc to stop editing") {
		t.Fatalf("editing help remained after Escape:\n%s", plain)
	}
}

func TestNotificationFilterAcceptsSpacesAndPrintableKeys(t *testing.T) {
	model := newNotificationManagerModel(context.Background(), nil, defaultPreferences(), 80, 24, colorPalette{})
	model.followed = []Followed{{ID: "1", Name: "Kimmy DNC"}, {ID: "2", Name: "Q Channel"}}
	model.filtering = true
	for _, key := range []string{"K", "i", "m", "m", "y", " ", "D", "N", "C"} {
		model, _ = pressNotificationManagerKey(model, key)
	}
	if model.filter != "Kimmy DNC" || len(model.filteredChannels()) != 1 || model.filteredChannels()[0].ID != "1" {
		t.Fatalf("filter=%q, matches=%+v", model.filter, model.filteredChannels())
	}
	model, _ = pressNotificationManagerKey(model, "enter")
	if model.filtering || len(model.prefs.NotificationChannels) != 0 {
		t.Fatalf("Enter should finish editing without toggling: filtering=%v channels=%v", model.filtering, model.prefs.NotificationChannels)
	}
	model, _ = pressNotificationManagerKey(model, "space")
	if len(model.prefs.NotificationChannels) != 1 || model.prefs.NotificationChannels[0] != "1" {
		t.Fatalf("Space outside filter mode did not toggle selected channel: %v", model.prefs.NotificationChannels)
	}

	model.filtering = true
	model, _ = pressNotificationManagerKey(model, "/")
	if model.filter != "Kimmy DNC/" {
		t.Fatalf("slash in filter mode was not text: %q", model.filter)
	}
}

func TestNotificationFilterQIsTextUntilEditingEnds(t *testing.T) {
	model := newNotificationManagerModel(context.Background(), nil, defaultPreferences(), 80, 24, colorPalette{})
	model.filtering = true
	model, closed := pressNotificationManagerKey(model, "q")
	if closed || !model.filtering || model.filter != "q" {
		t.Fatalf("q while filtering = filter:%q editing:%v closed:%v", model.filter, model.filtering, closed)
	}
	model, closed = pressNotificationManagerKey(model, "esc")
	if closed || model.filtering {
		t.Fatalf("Escape should finish filter editing: editing:%v closed:%v", model.filtering, closed)
	}
	model, closed = pressNotificationManagerKey(model, "q")
	if !closed {
		t.Fatal("q outside filter mode should quit the manager")
	}
}

func TestNotificationManagerShowsDistinctEmptyStates(t *testing.T) {
	model := newNotificationManagerModel(context.Background(), nil, defaultPreferences(), 80, 24, colorPalette{})
	plain := ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "You don't follow any channels.") {
		t.Fatalf("empty followed list message missing:\n%s", plain)
	}

	model.followed = []Followed{{ID: "1", Name: "KimmyDNC"}}
	model.filter = "missing"
	plain = ansi.Strip(model.View().Content)
	if !strings.Contains(plain, `No followed channels match "missing".`) {
		t.Fatalf("no filter matches message missing:\n%s", plain)
	}
}

func TestNotificationManagerHelpMatchesNavigationAndExitContext(t *testing.T) {
	model := newNotificationManagerModel(context.Background(), nil, defaultPreferences(), 120, 24, colorPalette{})
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"↑/k up", "↓/j down", "esc back"} {
		if !strings.Contains(plain, want) {
			t.Errorf("embedded manager help missing %q:\n%s", want, plain)
		}
	}
	model.standalone = true
	plain = ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "esc exit") {
		t.Errorf("standalone manager help should explain Escape exits:\n%s", plain)
	}
}

func TestNotificationManagerRetainsEditWhenSaveFails(t *testing.T) {
	prefs := defaultPreferences()
	model := newNotificationManagerModel(context.Background(), nil, prefs, 80, 20, colorPalette{})
	model.followed = []Followed{{ID: "channel-id", Name: "Channel", Login: "channel"}}
	model, _ = pressNotificationManagerKey(model, "space")
	if len(model.prefs.NotificationChannels) != 1 || model.prefs.NotificationChannels[0] != "channel-id" {
		t.Fatalf("unsaved selection was lost: %v", model.prefs.NotificationChannels)
	}
	if model.saveErr == nil {
		t.Fatal("expected a save error")
	}
	plain := ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "[x] Channel") || !strings.Contains(plain, "Could not save preferences") {
		t.Fatalf("save failure or retained selection was not shown: %s", plain)
	}
	if !strings.Contains(plain, "r retry save") {
		t.Fatalf("save retry action missing from help: %s", plain)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	model.prefs.ConfigPath = path
	model, _ = pressNotificationManagerKey(model, "r")
	if model.saveErr != nil {
		t.Fatalf("retry save failed: %v", model.saveErr)
	}
	if _, err := loadPreferences(path); err != nil {
		t.Fatalf("retry did not write valid preferences: %v", err)
	}
	if strings.Contains(ansi.Strip(model.View().Content), "r retry save") {
		t.Fatal("help still offers retry after successful save")
	}
}

func TestLiveModeDelegatesNotificationScreenAndReturns(t *testing.T) {
	model := newLiveModeModel(context.Background(), nil, liveModeOptions{interval: time.Minute, preferences: defaultPreferences(), initialScreen: liveScreen})
	updated, cmd := pressLiveModeKey(model, "n")
	if updated.screen != notificationManagerScreen || cmd == nil {
		t.Fatalf("manager open state = %v, command nil:%v", updated.screen, cmd == nil)
	}
	closedModel, cmd := updated.updateNotificationManager(tea.KeyPressMsg{Code: tea.KeyEsc})
	if closedModel.(liveModeModel).screen != liveScreen || cmd == nil {
		t.Fatalf("manager close did not return to live screen: model=%#v command=%v", closedModel, cmd)
	}

	model = newLiveModeModel(context.Background(), nil, liveModeOptions{preferences: defaultPreferences(), initialScreen: notificationManagerScreen})
	updatedModel, cmd := model.updateNotificationManager(tea.KeyPressMsg{Code: tea.KeyEsc})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("standalone manager close command = %T, want tea.QuitMsg", cmd())
	}
	if updatedModel.(liveModeModel).screen != notificationManagerScreen {
		t.Fatalf("standalone manager unexpectedly changed screen")
	}
}

func pressNotificationManagerKey(model notificationManagerModel, key string) (notificationManagerModel, bool) {
	var message tea.KeyPressMsg
	switch key {
	case "space":
		message.Code = tea.KeySpace
	case "esc":
		message.Code = tea.KeyEsc
	case "enter":
		message.Code = tea.KeyEnter
	default:
		message.Text = key
		if key != "" {
			message.Code = []rune(key)[0]
		}
	}
	updated, _, closed := model.Update(message)
	return updated, closed
}
