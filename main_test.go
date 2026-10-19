package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestLiveFollowedPaginatesAndBatchesStreams(t *testing.T) {
	var followPages, streamRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels/followed":
			followPages++
			if r.URL.Query().Get("user_id") != "me" {
				t.Errorf("user_id = %q", r.URL.Query().Get("user_id"))
			}
			if followPages == 1 {
				json.NewEncoder(w).Encode(map[string]any{"data": []Followed{{ID: "1"}}, "pagination": map[string]string{"cursor": "next"}})
			} else {
				json.NewEncoder(w).Encode(map[string]any{"data": []Followed{{ID: "2"}}, "pagination": map[string]string{}})
			}
		case "/streams":
			streamRequests++
			ids := r.URL.Query()["user_id"]
			if len(ids) == 0 {
				t.Error("streams request missing broadcaster IDs")
			}
			json.NewEncoder(w).Encode(map[string]any{"data": []Stream{{Login: "small", Viewers: 2}, {Login: "large", Viewers: 20}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	auth := &OAuth{ClientID: "client", Store: &CredentialStore{Disabled: true}, credentials: &Credentials{AccessToken: "access"}, validatedAt: time.Now(), userID: "me"}
	api := &Twitch{ClientID: "client", Auth: auth, BaseURL: server.URL}
	streams, err := api.LiveFollowed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if followPages != 2 {
		t.Fatalf("follow pages = %d, want 2", followPages)
	}
	if streamRequests != 1 {
		t.Fatalf("stream requests = %d, want 1", streamRequests)
	}
	if len(streams) != 2 || streams[0].Login != "large" {
		t.Fatalf("unexpected sorted streams: %+v", streams)
	}
}

func TestLiveFollowedBatchesOverHundredIDs(t *testing.T) {
	var streamQuerySizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/channels/followed" {
			data := make([]Followed, 201)
			for i := range data {
				data[i].ID = strconv.Itoa(i + 1)
			}
			json.NewEncoder(w).Encode(map[string]any{"data": data, "pagination": map[string]string{}})
			return
		}
		streamQuerySizes = append(streamQuerySizes, len(r.URL.Query()["user_id"]))
		json.NewEncoder(w).Encode(map[string]any{"data": []Stream{}})
	}))
	defer server.Close()
	auth := &OAuth{ClientID: "client", Store: &CredentialStore{Disabled: true}, credentials: &Credentials{AccessToken: "access"}, validatedAt: time.Now(), userID: "me"}
	api := &Twitch{ClientID: "client", Auth: auth, BaseURL: server.URL}
	if _, err := api.LiveFollowed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(streamQuerySizes) != "[100 100 1]" {
		t.Fatalf("stream query sizes = %v", streamQuerySizes)
	}
}

func TestSortStreamsKeysAndDirections(t *testing.T) {
	started := time.Now()
	streams := []Stream{
		{Login: "zeta", Name: "Zeta", Title: "Alpha", Game: "Beta", Viewers: 10, StartedAt: started.Add(-4 * time.Hour)},
		{Login: "alpha", Name: "Alpha", Title: "Gamma", Game: "Alpha", Viewers: 30, StartedAt: started.Add(-time.Hour)},
		{Login: "mu", Name: "Mu", Title: "Beta", Game: "Gamma", Viewers: 20, StartedAt: started.Add(-2 * time.Hour)},
	}
	for _, tc := range []struct {
		by, order string
		want      string
	}{
		{by: "viewers", order: "desc", want: "alpha,mu,zeta"},
		{by: "viewers", order: "asc", want: "zeta,mu,alpha"},
		{by: "channel", order: "asc", want: "alpha,mu,zeta"},
		{by: "title", order: "asc", want: "zeta,mu,alpha"},
		{by: "category", order: "asc", want: "alpha,zeta,mu"},
		{by: "live_duration", order: "desc", want: "zeta,mu,alpha"},
		{by: "live_duration", order: "asc", want: "alpha,mu,zeta"},
	} {
		t.Run(tc.by+"/"+tc.order, func(t *testing.T) {
			copy := append([]Stream(nil), streams...)
			sortStreams(copy, tc.by, tc.order)
			logins := make([]string, len(copy))
			for i, stream := range copy {
				logins[i] = stream.Login
			}
			if got := strings.Join(logins, ","); got != tc.want {
				t.Fatalf("order = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSortStreamsKeepsMissingLiveTimesLast(t *testing.T) {
	streams := []Stream{{Login: "unknown"}, {Login: "started", StartedAt: time.Now()}}
	sortStreams(streams, "live_duration", "desc")
	if streams[0].Login != "started" || streams[1].Login != "unknown" {
		t.Fatalf("sorted streams = %+v", streams)
	}
}

func TestRateLimitErrorIncludesReset(t *testing.T) {
	reset := time.Now().Add(time.Minute).Unix()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Ratelimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	auth := &OAuth{ClientID: "client", Store: &CredentialStore{Disabled: true}, credentials: &Credentials{AccessToken: "access"}, validatedAt: time.Now(), userID: "me"}
	api := &Twitch{ClientID: "client", Auth: auth, BaseURL: server.URL}
	var result map[string]any
	err := api.get(context.Background(), "/test", url.Values{}, &result)
	if err == nil || !strings.Contains(err.Error(), "try again after") {
		t.Fatalf("expected actionable rate limit error, got %v", err)
	}
}

func TestDurationAndWrap(t *testing.T) {
	if got := duration(90 * time.Minute); got != "1h 30m" {
		t.Fatalf("duration = %q", got)
	}
	if got := wrapText("one two three", 7); got != "one two\n  three" {
		t.Fatalf("wrapped text = %q", got)
	}
}

func TestFormatRefreshIntervalOmitsZeroUnits(t *testing.T) {
	for _, tc := range []struct {
		interval time.Duration
		want     string
	}{
		{interval: time.Minute, want: "1m"},
		{interval: time.Hour, want: "1h"},
		{interval: time.Hour + 30*time.Minute, want: "1h 30m"},
		{interval: 90 * time.Second, want: "1m 30s"},
		{interval: 500 * time.Millisecond, want: "500ms"},
		{interval: 0, want: "0s"},
	} {
		if got := formatRefreshInterval(tc.interval); got != tc.want {
			t.Errorf("formatRefreshInterval(%s) = %q, want %q", tc.interval, got, tc.want)
		}
	}
}

func TestRenderStreamTableIncludesAllColumnsAndValues(t *testing.T) {
	streams := []Stream{{
		Login:     "sample_login",
		Title:     "A stream title",
		Game:      "Some category",
		Viewers:   1234,
		StartedAt: time.Now().Add(-90 * time.Minute),
	}}
	got := renderStreamTable(streams, 100)
	for _, want := range []string{"Channel", "Title", "Category", "Viewers", "Live For", "sample_login", "A stream title", "Some category", "1,234", "1h 30m"} {
		if !strings.Contains(got, want) {
			t.Errorf("table output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderStreamTableTruncatesToTerminalWidth(t *testing.T) {
	got := renderStreamTable([]Stream{{
		Name:      "A very long channel name",
		Title:     "This is a very long stream title that should be shortened to fit the terminal",
		Game:      "A very long category name",
		Viewers:   99,
		StartedAt: time.Now(),
	}}, 54)
	if !strings.Contains(got, "…") {
		t.Fatalf("expected long cells to be truncated:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if width := lipgloss.Width(line); width > 54 {
			t.Errorf("table line width = %d, want at most 54: %q", width, line)
		}
	}
}

func TestTableRowBackgroundsAlternate(t *testing.T) {
	first := tableRowBackground(0)
	second := tableRowBackground(1)
	third := tableRowBackground(2)
	if first == second {
		t.Fatal("consecutive rows should use different backgrounds")
	}
	if first != third {
		t.Fatal("row backgrounds should repeat after two rows")
	}
}

func TestTableRowBackgroundsHaveGrayscaleContrast(t *testing.T) {
	first := tableRowBackground(0)
	second := tableRowBackground(1)
	for _, tc := range []struct {
		name        string
		first, next string
	}{
		{name: "light terminal", first: first.Light, next: second.Light},
		{name: "dark terminal", first: first.Dark, next: second.Dark},
	} {
		brightnessDifference := absInt(grayscaleBrightness(tc.first) - grayscaleBrightness(tc.next))
		if brightnessDifference < 30 {
			t.Errorf("%s stripe brightness difference = %d, want at least 30", tc.name, brightnessDifference)
		}
	}
}

func grayscaleBrightness(hex string) int {
	value, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	red := int(value >> 16 & 0xff)
	green := int(value >> 8 & 0xff)
	blue := int(value & 0xff)
	return (299*red + 587*green + 114*blue) / 1000
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func TestCardRailColorsAlternate(t *testing.T) {
	first := cardRailColor(0)
	second := cardRailColor(1)
	third := cardRailColor(2)
	if first == second {
		t.Fatal("consecutive cards should use different rail colors")
	}
	if first != third {
		t.Fatal("card rail colors should repeat after two cards")
	}
}

func TestRenderStreamTableEmptyState(t *testing.T) {
	if got := renderStreamTable(nil, 80); !strings.Contains(got, "Nobody you follow is live") {
		t.Fatalf("unexpected empty state: %q", got)
	}
}

func TestRenderStreamCardsPrefixesEveryLineAndFitsWidth(t *testing.T) {
	got := renderStreamCards([]Stream{
		{Name: "Channel One", Title: "A wrapped stream title with several words", Game: "Game", Viewers: 123},
		{Name: "Channel Two", Title: "A second stream", Game: "Another game", Viewers: 45},
	}, 24)
	for _, line := range strings.Split(got, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "│") {
			t.Errorf("card line missing left rail: %q", line)
		}
		if width := lipgloss.Width(line); width > 24 {
			t.Errorf("card line width = %d, want at most 24: %q", width, line)
		}
	}
	if strings.Count(got, "│") < 6 {
		t.Fatalf("expected rails on card headers, wrapped text, and details:\n%s", got)
	}
}

func TestRenderStreamCardsTruncatesLongUnbrokenText(t *testing.T) {
	got := renderStreamCards([]Stream{{
		Name:    "ThisIsAnExtremelyLongChannelName",
		Title:   "ThisIsAnExtremelyLongTitleWithoutSpaces",
		Game:    "ThisIsAnExtremelyLongCategoryWithoutSpaces",
		Viewers: 987654321,
	}}, 20)
	for _, line := range strings.Split(got, "\n") {
		if width := lipgloss.Width(line); width > 20 {
			t.Errorf("card line width = %d, want at most 20: %q", width, line)
		}
	}
}

func TestSelectionAwareRenderersReturnItemSpans(t *testing.T) {
	streams := []Stream{
		{Login: "first", Title: "A short title", Game: "Game"},
		{Login: "second", Title: "A wrapped title with several words", Game: "Another game", Viewers: 123},
	}

	table, tableSpans := renderStreamTableWithSelection(streams, 80, 1)
	if len(tableSpans) != len(streams) || tableSpans[0] != (streamLineSpan{start: 2, end: 3}) || tableSpans[1] != (streamLineSpan{start: 3, end: 4}) {
		t.Fatalf("table spans = %+v", tableSpans)
	}
	if !strings.Contains(table, "second") || selectedItemBackground == tableRowBackground(1) {
		t.Fatal("selected table row should use its distinct highlight")
	}

	cards, cardSpans := renderStreamCardsWithSelection(streams, 24, 1)
	if len(cardSpans) != len(streams) || cardSpans[1].end-cardSpans[1].start < 4 {
		t.Fatalf("card spans = %+v, want wrapped lines in selected card", cardSpans)
	}
	if !strings.Contains(cards, "┃") {
		t.Fatalf("selected card should use a heavier left rail:\n%s", cards)
	}
	if !strings.Contains(cards, "123 viewers") {
		t.Fatalf("selected card should preserve the viewer count inside the highlight:\n%s", cards)
	}
	for _, line := range strings.Split(cards, "\n") {
		if lipgloss.Width(line) > 24 {
			t.Errorf("card line width = %d, want at most 24: %q", lipgloss.Width(line), line)
		}
	}
}

func TestSelectedCardLineFillsWidthAroundViewerCount(t *testing.T) {
	line := selectedCardLine("Channel  123 viewers", 40)
	if width := lipgloss.Width(line); width != 40 {
		t.Fatalf("selected line width = %d, want 40", width)
	}
	if !strings.Contains(line, "Channel  123 viewers") {
		t.Fatalf("selected line lost channel or viewer text: %q", line)
	}
}

func TestJSONAndDisplayCannotBeCombined(t *testing.T) {
	err := run([]string{"--json", "--display", "card"})
	if err == nil || !strings.Contains(err.Error(), "--json and --display cannot be used together") {
		t.Fatalf("expected conflicting format error, got %v", err)
	}
}

func TestUsagePutsClientIDUnderDeveloperOptions(t *testing.T) {
	var output strings.Builder
	printUsage(&output)
	help := output.String()
	developerOptions := strings.Index(help, "Developer options:")
	clientID := strings.Index(help, "--client-id ID")
	if developerOptions < 0 || clientID < developerOptions {
		t.Fatalf("client ID option should appear under developer options:\n%s", help)
	}
	for _, option := range []string{"--display MODE", "--sort-by FIELD", "--sort-order ORDER", "--refresh-interval DURATION", "--once", "--config PATH"} {
		if !strings.Contains(help, option) {
			t.Errorf("help missing %q:\n%s", option, help)
		}
	}
	if strings.Contains(help, "--card") || strings.Contains(help, "--live-mode") {
		t.Fatalf("help should not expose retired display/live-mode flags:\n%s", help)
	}
}

func TestShouldRunLiveMode(t *testing.T) {
	for _, tc := range []struct {
		name        string
		once        bool
		jsonOutput  bool
		openLogin   string
		interactive bool
		want        bool
	}{
		{name: "interactive defaults to live mode", interactive: true, want: true},
		{name: "once opts out", once: true, interactive: true},
		{name: "json is one shot", jsonOutput: true, interactive: true},
		{name: "open is one shot", openLogin: "channel", interactive: true},
		{name: "noninteractive falls back to snapshot", interactive: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRunLiveMode(tc.once, tc.jsonOutput, tc.openLogin, tc.interactive); got != tc.want {
				t.Fatalf("shouldRunLiveMode() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLiveModeSelectionNavigation(t *testing.T) {
	model := testLiveModeModel("table", []Stream{{Login: "one"}, {Login: "two"}, {Login: "three"}}, 0, 80, 12)
	for _, tc := range []struct {
		key  string
		want int
	}{
		{key: "down", want: 1},
		{key: "j", want: 2},
		{key: "down", want: 2},
		{key: "up", want: 1},
		{key: "k", want: 0},
		{key: "end", want: 2},
		{key: "home", want: 0},
	} {
		model, _ = pressLiveModeKey(model, tc.key)
		if model.selected != tc.want {
			t.Errorf("after %s selected = %d, want %d", tc.key, model.selected, tc.want)
		}
	}
}

func TestReconcileSelectedStream(t *testing.T) {
	previous := []Stream{{Login: "one"}, {Login: "two"}, {Login: "three"}}
	if got := reconcileSelectedStream(previous, 1, []Stream{{Login: "three"}, {Login: "TWO"}, {Login: "one"}}); got != 1 {
		t.Fatalf("selected index after reorder = %d, want 1", got)
	}
	if got := reconcileSelectedStream(previous, 2, []Stream{{Login: "one"}, {Login: "two"}}); got != 1 {
		t.Fatalf("selected index after removal = %d, want clamped index 1", got)
	}
	if got := reconcileSelectedStream(previous, 1, nil); got != -1 {
		t.Fatalf("selected index after empty refresh = %d, want -1", got)
	}
}

func TestLiveModeRefreshPreservesSelectedLogin(t *testing.T) {
	model := testLiveModeModel("table", []Stream{{Login: "one"}, {Login: "two"}, {Login: "three"}}, 1, 80, 12)
	updated, cmd := model.Update(liveModeResultMsg{streams: []Stream{{Login: "two"}, {Login: "three"}, {Login: "one"}}})
	if cmd == nil {
		t.Fatal("successful refresh should schedule the next refresh")
	}
	got := updated.(liveModeModel)
	if got.selected != 0 || got.streams[got.selected].Login != "two" {
		t.Fatalf("selected stream after refresh = %+v", got.streams[got.selected])
	}
}

func TestLiveModeSelectionScrollsWrappedCardsIntoView(t *testing.T) {
	streams := make([]Stream, 12)
	for i := range streams {
		streams[i] = Stream{Login: fmt.Sprintf("channel-%02d", i), Title: "a long stream title with several words", Game: "Game"}
	}
	model := testLiveModeModel("card", streams, 0, 24, 10)
	model.selectStream(8)
	_, spans := model.renderedStreamList(24)
	span := spans[model.selected]
	if span.start < model.scroll || span.end > model.scroll+model.bodyHeight() {
		t.Fatalf("selected card span %+v is outside viewport scroll=%d height=%d", span, model.scroll, model.bodyHeight())
	}
	if model.scroll > model.maxScroll() {
		t.Fatalf("scroll = %d, max scroll = %d", model.scroll, model.maxScroll())
	}
}

func TestLiveModeEnterOpensSelectedStreamAndKeepsScreenOpen(t *testing.T) {
	previousLauncher := browserLauncher
	t.Cleanup(func() { browserLauncher = previousLauncher })
	var openedURL string
	browserLauncher = func(url string) error {
		openedURL = url
		return nil
	}
	model := testLiveModeModel("table", []Stream{{Login: "first"}, {Login: "selected"}}, 1, 80, 12)
	updated, cmd := pressLiveModeKey(model, "enter")
	if cmd == nil {
		t.Fatal("Enter should launch the selected stream")
	}
	result, ok := cmd().(streamOpenResultMsg)
	if !ok || result.err != nil {
		t.Fatalf("open command result = %#v", result)
	}
	if openedURL != "https://www.twitch.tv/selected" {
		t.Fatalf("opened URL = %q", openedURL)
	}
	openedModel, quit := updated.Update(result)
	if quit != nil {
		t.Fatal("handling a successful open should not quit the live screen")
	}
	if openedModel.(liveModeModel).openErr != nil {
		t.Fatalf("unexpected open error: %v", openedModel.(liveModeModel).openErr)
	}
}

func TestLiveModeEnterReportsOpenFailure(t *testing.T) {
	previousLauncher := browserLauncher
	t.Cleanup(func() { browserLauncher = previousLauncher })
	browserLauncher = func(string) error { return errors.New("browser unavailable") }
	model := testLiveModeModel("table", []Stream{{Login: "channel"}}, 0, 80, 12)
	updated, cmd := pressLiveModeKey(model, "enter")
	result := cmd().(streamOpenResultMsg)
	openedModel, quit := updated.Update(result)
	if quit != nil {
		t.Fatal("handling an open failure should not quit the live screen")
	}
	if !strings.Contains(openedModel.(liveModeModel).View(), "open failed: browser unavailable") {
		t.Fatal("live screen should show the browser launch error")
	}
}

func TestLiveModeEnterOnEmptyListDoesNothing(t *testing.T) {
	model := testLiveModeModel("table", nil, -1, 80, 12)
	updated, cmd := pressLiveModeKey(model, "enter")
	if cmd != nil || updated.selected != -1 {
		t.Fatalf("empty-list Enter changed model or returned command: cmd=%v model=%+v", cmd, updated)
	}
}

func testLiveModeModel(display string, streams []Stream, selected, width, height int) liveModeModel {
	return liveModeModel{
		ctx:       context.Background(),
		interval:  time.Minute,
		display:   display,
		streams:   streams,
		selected:  selected,
		hasResult: true,
		width:     width,
		height:    height,
	}
}

func pressLiveModeKey(model liveModeModel, key string) (liveModeModel, tea.Cmd) {
	var message tea.KeyMsg
	switch key {
	case "enter":
		message.Type = tea.KeyEnter
	case "up":
		message.Type = tea.KeyUp
	case "down":
		message.Type = tea.KeyDown
	case "home":
		message.Type = tea.KeyHome
	case "end":
		message.Type = tea.KeyEnd
	default:
		message.Type = tea.KeyRunes
		message.Runes = []rune(key)
	}
	updated, cmd := model.Update(message)
	return updated.(liveModeModel), cmd
}

func TestBrowserLauncherReceivesChannelURL(t *testing.T) {
	previous := browserLauncher
	defer func() { browserLauncher = previous }()
	var opened string
	browserLauncher = func(url string) error { opened = url; return nil }
	if err := openBrowser("https://www.twitch.tv/sample"); err != nil {
		t.Fatal(err)
	}
	if opened != "https://www.twitch.tv/sample" {
		t.Fatalf("opened URL = %q", opened)
	}
}

func TestOAuthValidationRequiresScopeAndMatchingClient(t *testing.T) {
	for _, tc := range []struct {
		name, client string
		scopes       []string
		want         string
	}{
		{name: "ok", client: "client", scopes: []string{scopeFollows}},
		{name: "wrong client", client: "other", scopes: []string{scopeFollows}, want: "different Twitch client"},
		{name: "missing scope", client: "client", scopes: []string{}, want: "missing user:read:follows"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "OAuth secret" {
					t.Errorf("unexpected authorization header")
				}
				json.NewEncoder(w).Encode(map[string]any{"client_id": tc.client, "user_id": "user", "scopes": tc.scopes})
			}))
			defer server.Close()
			o := &OAuth{ClientID: "client", BaseURL: server.URL, HTTP: server.Client(), credentials: &Credentials{AccessToken: "secret"}}
			err := o.validate(context.Background())
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("validate error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if o.userID != "user" {
				t.Fatalf("user ID = %q", o.userID)
			}
		})
	}
}

func TestOAuthValidatesThenRotatesPublicRefreshToken(t *testing.T) {
	validated := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/validate":
			validated++
			if r.Header.Get("Authorization") == "OAuth old-access" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"client_id": "client", "user_id": "user", "scopes": []string{scopeFollows}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("grant_type") != "refresh_token" {
				t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
			}
			if r.Form.Get("client_secret") != "" {
				t.Error("public client refresh included a secret")
			}
			if r.Form.Get("refresh_token") != "refresh-old" {
				t.Errorf("refresh token = %q", r.Form.Get("refresh_token"))
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "new-access", "refresh_token": "refresh-new"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	o := &OAuth{ClientID: "client", BaseURL: server.URL, HTTP: server.Client(), Store: &CredentialStore{Disabled: true}, credentials: &Credentials{AccessToken: "old-access", RefreshToken: "refresh-old"}, validatedAt: time.Now().Add(-2 * time.Hour)}
	token, err := o.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "new-access" {
		t.Fatalf("token = %q", token)
	}
	if o.credentials.RefreshToken != "refresh-new" {
		t.Fatalf("refresh token was not rotated: %q", o.credentials.RefreshToken)
	}
	if validated != 2 {
		t.Fatalf("validation requests = %d, want 2", validated)
	}
}
