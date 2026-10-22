package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestOAuthValidationLogsToConfiguredWriter(t *testing.T) {
	var logs bytes.Buffer
	oauth := &OAuth{
		ClientID: "client", BaseURL: "https://twitch.invalid", LogWriter: &logs,
		HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"client_id":"client","scopes":["user:read:follows"],"user_id":"viewer"}`)), Header: make(http.Header)}, nil
		})},
		credentials: &Credentials{AccessToken: "access"}, validatedAt: time.Now().Add(-time.Hour),
	}
	if _, err := oauth.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "saved access token is valid") {
		t.Fatalf("validation log missing: %q", logs.String())
	}

	logs.Reset()
	oauth.LogWriter = io.Discard
	oauth.validatedAt = time.Now().Add(-time.Hour)
	if _, err := oauth.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Fatalf("discarded auth log was written: %q", logs.String())
	}
}

func TestOAuthAuthLogDefaultsToStderr(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = writer
	(&OAuth{}).authLog("default destination")
	os.Stderr = previous
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if _, err := output.ReadFrom(reader); err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	if !strings.Contains(output.String(), "default destination") {
		t.Fatalf("stderr log missing: %q", output.String())
	}
}

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

func TestLiveFollowedSubscriptionsAreOptInAndCached(t *testing.T) {
	var subscriptionRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels/followed":
			json.NewEncoder(w).Encode(map[string]any{"data": []Followed{{ID: "broadcaster"}}, "pagination": map[string]string{}})
		case "/streams":
			json.NewEncoder(w).Encode(map[string]any{"data": []Stream{{UserID: "broadcaster", Login: "channel", Viewers: 10}}})
		case "/subscriptions/user":
			subscriptionRequests++
			if r.URL.Query().Get("user_id") != "viewer" || r.URL.Query().Get("broadcaster_id") != "broadcaster" {
				t.Errorf("subscription query = %v", r.URL.Query())
			}
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"tier": "2000"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	auth := &OAuth{ClientID: "client", Store: &CredentialStore{Disabled: true}, credentials: &Credentials{AccessToken: "access"}, validatedAt: time.Now(), userID: "viewer"}
	api := &Twitch{ClientID: "client", Auth: auth, HTTP: server.Client(), BaseURL: server.URL}
	streams, err := api.LiveFollowedSorted(context.Background(), "viewers", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if subscriptionRequests != 0 || streams[0].Subscription != nil {
		t.Fatalf("subscription data should be absent by default: calls=%d stream=%+v", subscriptionRequests, streams[0])
	}
	api.Subscriptions = true
	for i := 0; i < 2; i++ {
		streams, err = api.LiveFollowedSorted(context.Background(), "viewers", "desc")
		if err != nil {
			t.Fatal(err)
		}
		if streams[0].Subscription == nil || !streams[0].Subscription.Subscribed || streams[0].Subscription.Tier != 2000 {
			t.Fatalf("subscription metadata = %+v", streams[0].Subscription)
		}
	}
	if subscriptionRequests != 1 {
		t.Fatalf("subscription requests = %d, want cached single request", subscriptionRequests)
	}
}

func TestSubscriptionNotFoundMeansNotSubscribed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	auth := &OAuth{ClientID: "client", Store: &CredentialStore{Disabled: true}, credentials: &Credentials{AccessToken: "access"}, validatedAt: time.Now(), userID: "viewer"}
	api := &Twitch{ClientID: "client", Auth: auth, HTTP: server.Client(), BaseURL: server.URL}
	info, err := api.subscription(context.Background(), "viewer", "broadcaster")
	if err != nil {
		t.Fatal(err)
	}
	if info.Subscribed || info.Tier != 0 {
		t.Fatalf("subscription = %+v, want not subscribed", info)
	}
}

func TestNotSubscribedResultsAreCached(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	auth := &OAuth{ClientID: "client", Store: &CredentialStore{Disabled: true}, credentials: &Credentials{AccessToken: "access"}, validatedAt: time.Now(), userID: "viewer"}
	api := &Twitch{ClientID: "client", Auth: auth, HTTP: server.Client(), BaseURL: server.URL}
	for i := 0; i < 2; i++ {
		streams := []Stream{{Login: "channel", UserID: "broadcaster"}}
		if err := api.enrichSubscriptions(context.Background(), "viewer", streams); err != nil {
			t.Fatal(err)
		}
		if streams[0].Subscription == nil || streams[0].Subscription.Subscribed {
			t.Fatalf("subscription = %+v, want cached not-subscribed result", streams[0].Subscription)
		}
	}
	if requests != 1 {
		t.Fatalf("subscription requests = %d, want 1", requests)
	}
}

func TestSubscriptionUnexpectedAPIErrorIsNotUnsubscribed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not eligible", http.StatusUnprocessableEntity)
	}))
	defer server.Close()
	auth := &OAuth{ClientID: "client", Store: &CredentialStore{Disabled: true}, credentials: &Credentials{AccessToken: "access"}, validatedAt: time.Now(), userID: "viewer"}
	api := &Twitch{ClientID: "client", Auth: auth, HTTP: server.Client(), BaseURL: server.URL}
	info, err := api.subscription(context.Background(), "viewer", "broadcaster")
	if err == nil || info.Subscribed {
		t.Fatalf("subscription = %+v, error = %v, want API error", info, err)
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

func TestSortStreamsSubscriptionStatusAndTier(t *testing.T) {
	streams := []Stream{
		{Login: "zed", Subscription: &SubscriptionInfo{Subscribed: true, Tier: 1000}},
		{Login: "alpha", Subscription: &SubscriptionInfo{Subscribed: true, Tier: 3000}},
		{Login: "mu", Subscription: &SubscriptionInfo{Subscribed: false}},
		{Login: "beta"},
	}
	for _, tc := range []struct {
		by, order string
		want      string
	}{
		{by: "subscribed", order: "desc", want: "alpha,zed,beta,mu"},
		{by: "subscribed", order: "asc", want: "beta,mu,alpha,zed"},
		{by: "sub_tier", order: "desc", want: "alpha,zed,beta,mu"},
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

func TestRenderStreamTableAlignsEmojiUsingWcWidth(t *testing.T) {
	streams := []Stream{
		{Login: "ordinary", Title: "A normal stream title", Game: "Art", Viewers: 902},
		{Login: "emoji", Title: "⏹️ Dragon's Dogma 2 DANK ARIS", Game: "Dragon's Dogma II", Viewers: 12},
		{Login: "family", Title: "👨‍👩‍👧‍👦 Family stream", Game: "Just Chatting", Viewers: 3},
	}
	got := renderStreamTable(streams, 80)
	lines := strings.Split(got, "\n")
	wantWidth := ansi.StringWidthWc(ansi.Strip(lines[0]))
	for i, line := range lines[1:] {
		if line == "" {
			continue
		}
		if width := ansi.StringWidthWc(ansi.Strip(line)); width != wantWidth {
			t.Errorf("table line %d has wcwidth %d, want %d: %q", i+2, width, wantWidth, ansi.Strip(line))
		}
	}
	if !strings.Contains(got, "⏹️") || !strings.Contains(got, "👨‍👩‍👧‍👦") {
		t.Fatalf("table should preserve emoji titles:\n%s", got)
	}
}

func TestFitTableCellTruncatesAtEmojiWidthBoundary(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width int
	}{
		{text: "⏹️ Dragon's Dogma", width: 1},
		{text: "⏹️ Dragon's Dogma", width: 2},
		{text: "👨‍👩‍👧‍👦 Family", width: 5},
		{text: "Plain text", width: 4},
	} {
		got := fitTableCell(tc.text, tc.width)
		if width := ansi.StringWidthWc(got); width != tc.width {
			t.Errorf("fitTableCell(%q, %d) width = %d, want %d; result %q", tc.text, tc.width, width, tc.width, got)
		}
	}
}

func TestThemePalettesAndNeutralSnapshot(t *testing.T) {
	light := lightPalette()
	if got := light.title; got != lipgloss.Color("#6D28D9") {
		t.Errorf("light title = %v", got)
	}
	streams := []Stream{{Login: "one", Name: "One", Title: "Title", Game: "Game", Viewers: 12}}
	neutral := renderStreamTableWithPalette(streams, 80, -1, colorPalette{})
	if strings.Contains(neutral, "\x1b[48;2;") || strings.Contains(neutral, "\x1b[48;5;") {
		t.Fatalf("neutral snapshot contains a filled background: %q", neutral)
	}
}

func TestLiveRenderersLeaveTerminalBackgroundVisible(t *testing.T) {
	streams := []Stream{{Name: "Channel", Title: "Title", Game: "Game", Viewers: 12}}
	for _, palette := range []colorPalette{darkPalette(), lightPalette()} {
		for name, rendered := range map[string]string{
			"table": renderStreamTableWithPalette(streams, 80, 0, palette),
			"cards": renderStreamCardsWithPalette(streams, 80, 0, palette),
		} {
			if strings.Contains(rendered, "\x1b[48") {
				t.Errorf("%s renderer set a background color: %q", name, rendered)
			}
		}
	}
}

func TestBackgroundColorSelectsLivePalette(t *testing.T) {
	model := liveModeModel{spinner: newLoadingSpinner(colorPalette{})}
	updated, _ := model.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#FFFFFF")})
	if got := updated.(liveModeModel).palette; got.title != lightPalette().title {
		t.Fatalf("light background selected palette with title %v", got.title)
	}
	updated, _ = model.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#000000")})
	if got := updated.(liveModeModel).palette; got.title != darkPalette().title {
		t.Fatalf("dark background selected palette with title %v", got.title)
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
	plain := ansi.Strip(got)
	for _, line := range strings.Split(plain, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			t.Errorf("card line missing left rail: %q", line)
		}
		if width := lipgloss.Width(line); width > 24 {
			t.Errorf("card line width = %d, want at most 24: %q", width, line)
		}
	}
	if strings.Contains(plain, "│") {
		t.Fatalf("unselected cards should not display a selection indicator:\n%s", got)
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
	if !strings.Contains(table, "second") || !strings.Contains(ansi.Strip(table), "│ second") {
		t.Fatal("selected table row should have a left selection indicator")
	}

	cards, cardSpans := renderStreamCardsWithSelection(streams, 24, 1)
	if len(cardSpans) != len(streams) || cardSpans[1].end-cardSpans[1].start < 4 {
		t.Fatalf("card spans = %+v, want wrapped lines in selected card", cardSpans)
	}
	if !strings.Contains(cards, "│") {
		t.Fatalf("selected card should have a left selection indicator:\n%s", cards)
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

func TestSubscriptionRenderingAndJSONAreOptIn(t *testing.T) {
	ordinary := []Stream{{Login: "channel", Name: "Channel", Viewers: 10}}
	if table := renderStreamTable(ordinary, 100); strings.Contains(table, "Subscription") {
		t.Fatalf("default table unexpectedly includes subscriptions:\n%s", table)
	}
	if cards := renderStreamCards(ordinary, 80); strings.Contains(cards, "Subscription") {
		t.Fatalf("default cards unexpectedly include subscriptions:\n%s", cards)
	}
	defaultJSON, err := json.Marshal(ordinary[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(defaultJSON), "subscription") {
		t.Fatalf("default JSON unexpectedly includes subscription data: %s", defaultJSON)
	}

	enriched := []Stream{{Login: "channel", Name: "Channel", Viewers: 10, Subscription: &SubscriptionInfo{Subscribed: true, Tier: 2000}}}
	if table := renderStreamTable(enriched, 120); !strings.Contains(table, "Subscription") || !strings.Contains(table, "Tier 2") {
		t.Fatalf("opt-in table missing subscription tier:\n%s", table)
	}
	if cards := renderStreamCards(enriched, 80); !strings.Contains(cards, "Subscription: Tier 2") {
		t.Fatalf("opt-in card missing subscription tier:\n%s", cards)
	}
	enrichedJSON, err := json.Marshal(enriched[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enrichedJSON), `"subscribed":true`) || !strings.Contains(string(enrichedJSON), `"tier":2000`) {
		t.Fatalf("opt-in JSON missing subscription data: %s", enrichedJSON)
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

func TestOpenCannotBeCombinedWithOutputFormatOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--open", "channel", "--once"},
		{"--open", "channel", "--json"},
		{"--open", "channel", "--display", "table"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			err := run(args)
			if err == nil || !strings.Contains(err.Error(), "--open cannot be combined") {
				t.Fatalf("expected --open format conflict, got %v", err)
			}
		})
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
	for _, option := range []string{"--display MODE", "--sort-by FIELD", "--sort-order ORDER", "--refresh-interval DURATION", "--subscriptions", "--viewer MODE", "--player NAME", "--chat", "--quality NAME", "--low-latency", "--once", "--config PATH"} {
		if !strings.Contains(help, option) {
			t.Errorf("help missing %q:\n%s", option, help)
		}
	}
	if strings.Contains(help, "--card") || strings.Contains(help, "--live-mode") {
		t.Fatalf("help should not expose retired display/live-mode flags:\n%s", help)
	}
	authLog := strings.Index(help, "--auth-log PATH")
	if authLog < developerOptions {
		t.Fatalf("auth log option should appear under Developer options:\n%s", help)
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

func TestAuthLogDestination(t *testing.T) {
	logFile := &bytes.Buffer{}
	for _, tc := range []struct {
		name           string
		snapshotOutput bool
		file           io.Writer
		want           io.Writer
	}{
		{name: "snapshot suppresses diagnostics", snapshotOutput: true, want: io.Discard},
		{name: "auth log file takes precedence for snapshot", snapshotOutput: true, file: logFile, want: logFile},
		{name: "auth log file captures diagnostics outside snapshot", file: logFile, want: logFile},
		{name: "default destination outside snapshot output", want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := authLogDestination(tc.snapshotOutput, tc.file); got != tc.want {
				t.Fatalf("authLogDestination() = %v, want %v", got, tc.want)
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

func TestLiveModeHeaderAlignmentAndFooterPlacement(t *testing.T) {
	model := testLiveModeModel("table", []Stream{{Name: "Channel", Title: "Title", Game: "Game"}}, 0, 80, 12)
	model.palette = darkPalette()
	model.lastUpdate = time.Date(2026, time.October, 19, 14, 5, 6, 0, time.UTC)
	view := model.View().Content
	plain := ansi.Strip(view)
	lines := strings.Split(plain, "\n")
	if !strings.HasPrefix(lines[0], "  LIVE FOLLOWED CHANNELS") {
		t.Fatalf("header should align with the table gutter: %q", lines[0])
	}
	if !strings.Contains(lines[0], " · updated 14:05:06") {
		t.Fatalf("last update should appear beside the header in lowercase: %q", lines[0])
	}
	refreshLine := strings.Index(plain, "Refreshes automatically every 1m")
	helpLine := strings.Index(plain, "↑/↓ select")
	channelLine := strings.Index(plain, "Channel")
	if refreshLine < channelLine || helpLine < refreshLine {
		t.Fatalf("refresh interval and help should follow the stream list:\n%s", plain)
	}
	if !strings.HasSuffix(plain, "↑/↓ select · enter open · pgup/pgdn scroll · r refresh · q quit") {
		t.Fatalf("help should be the last line in the display:\n%s", plain)
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
	result := streamOpenCommandResult(t, cmd)
	if result.err != nil {
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
	result := streamOpenCommandResult(t, cmd)
	openedModel, quit := updated.Update(result)
	if quit != nil {
		t.Fatal("handling an open failure should not quit the live screen")
	}
	if !strings.Contains(openedModel.(liveModeModel).View().Content, "open failed: browser unavailable") {
		t.Fatal("live screen should show the browser launch error")
	}
}

func TestLiveModeShowsSpinnerWhileFetching(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hasResult bool
		want      string
	}{
		{name: "initial load", hasResult: false, want: "⠉ Loading followed channels"},
		{name: "refresh", hasResult: true, want: "⠉ updating"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := testLiveModeModel("table", nil, -1, 80, 12)
			model.hasResult = tc.hasResult
			model.fetching = true
			model.spinner = newLoadingSpinner(model.palette)
			model.spinnerTicking = true

			updated, cmd := model.Update(model.spinner.Tick())
			if cmd == nil {
				t.Fatal("active fetch should schedule the next spinner frame")
			}
			if got := ansi.Strip(updated.(liveModeModel).View().Content); !strings.Contains(got, tc.want) {
				t.Fatalf("fetching view missing spinner text %q:\n%s", tc.want, got)
			}
		})
	}
}

func streamOpenCommandResult(t *testing.T, cmd tea.Cmd) streamOpenResultMsg {
	t.Helper()
	message := cmd()
	if batch, ok := message.(tea.BatchMsg); ok {
		for _, batchedCmd := range batch {
			if result, ok := batchedCmd().(streamOpenResultMsg); ok {
				return result
			}
		}
	}
	if result, ok := message.(streamOpenResultMsg); ok {
		return result
	}
	t.Fatalf("open command produced %T, want streamOpenResultMsg", message)
	return streamOpenResultMsg{}
}

func TestLiveModeKeepsRunningWithMultipleStreamlinkPlayers(t *testing.T) {
	var started []*exec.Cmd
	model := testLiveModeModel("table", []Stream{{Login: "first"}, {Login: "second"}}, 0, 80, 12)
	model.viewer = viewerOptions{Viewer: "streamlink"}
	model.startStream = func(cmd *exec.Cmd) error {
		if cmd.Stdin != nil || cmd.Stdout != nil || cmd.Stderr != nil {
			t.Fatal("Streamlink process should not use the TUI terminal")
		}
		started = append(started, cmd)
		return nil
	}
	model.waitStream = func(*exec.Cmd) error { return nil }

	first, firstWait := model.Update(viewerPreparedMsg{channel: "first", plan: launchPlan{stream: &exec.Cmd{Args: []string{"streamlink", "first"}}}})
	if firstWait == nil {
		t.Fatal("started Streamlink process should have a wait command")
	}
	model = first.(liveModeModel)
	if model.launching || model.activeStreams != 1 {
		t.Fatalf("model after first launch = launching:%v active:%d", model.launching, model.activeStreams)
	}
	if !strings.Contains(model.View().Content, "1 Streamlink player running") {
		t.Fatal("TUI should show active playback status")
	}

	model.selected = 1
	model.launching = true
	second, secondWait := model.Update(viewerPreparedMsg{channel: "second", plan: launchPlan{stream: &exec.Cmd{Args: []string{"streamlink", "second"}}}})
	if secondWait == nil {
		t.Fatal("second Streamlink process should also have a wait command")
	}
	model = second.(liveModeModel)
	if model.activeStreams != 2 || len(started) != 2 {
		t.Fatalf("active Streamlink processes = %d, started = %d; want 2", model.activeStreams, len(started))
	}

	finished, refresh := model.Update(firstWait().(streamFinishedMsg))
	if refresh != nil {
		t.Fatal("process completion should not replace the refresh schedule")
	}
	model = finished.(liveModeModel)
	if model.activeStreams != 1 || model.launching {
		t.Fatalf("model after one player exits = launching:%v active:%d", model.launching, model.activeStreams)
	}

	refreshed, nextRefresh := model.Update(liveModeResultMsg{streams: model.streams})
	if nextRefresh == nil || refreshed.(liveModeModel).activeStreams != 1 {
		t.Fatal("TUI refresh should continue while a Streamlink player is active")
	}
}

func TestLiveModeReportsStreamlinkStartAndExitFailures(t *testing.T) {
	model := testLiveModeModel("table", []Stream{{Login: "channel"}}, 0, 80, 12)
	model.viewer = viewerOptions{Viewer: "streamlink"}
	model.startStream = func(*exec.Cmd) error { return errors.New("process unavailable") }
	model.launching = true
	updated, cmd := model.Update(viewerPreparedMsg{channel: "channel", plan: launchPlan{stream: &exec.Cmd{}}})
	if cmd != nil {
		t.Fatal("failed start should not schedule process wait")
	}
	model = updated.(liveModeModel)
	if model.launching || model.activeStreams != 0 || model.openErr == nil || !strings.Contains(model.openErr.Error(), "start Streamlink for channel: process unavailable") {
		t.Fatalf("start failure state = %+v", model)
	}

	model.activeStreams = 1
	finished, _ := model.Update(streamFinishedMsg{channel: "channel", err: fmt.Errorf("run Streamlink: %w", errors.New("exit status 1"))})
	model = finished.(liveModeModel)
	if model.activeStreams != 0 || model.openErr == nil || !strings.Contains(model.openErr.Error(), "Streamlink for channel: run Streamlink: exit status 1") {
		t.Fatalf("exit failure state = %+v", model)
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
	var message tea.KeyPressMsg
	switch key {
	case "enter":
		message.Code = tea.KeyEnter
	case "up":
		message.Code = tea.KeyUp
	case "down":
		message.Code = tea.KeyDown
	case "home":
		message.Code = tea.KeyHome
	case "end":
		message.Code = tea.KeyEnd
	default:
		message.Text = key
		if len(key) > 0 {
			message.Code = []rune(key)[0]
		}
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

func TestOAuthRequestedScopesKeepSubscriptionsOptional(t *testing.T) {
	base := &OAuth{}
	if got := strings.Join(base.requestedScopes(), " "); got != scopeFollows {
		t.Fatalf("default scopes = %q, want %q", got, scopeFollows)
	}
	withSubscriptions := &OAuth{Subscriptions: true}
	want := scopeFollows + " " + scopeSubscriptions
	if got := strings.Join(withSubscriptions.requestedScopes(), " "); got != want {
		t.Fatalf("subscription scopes = %q, want %q", got, want)
	}
}

func TestOAuthSubscriptionUpgradePreservesCredentialsWhenCancelled(t *testing.T) {
	previousLauncher := browserLauncher
	browserLauncher = func(string) error { return nil }
	t.Cleanup(func() { browserLauncher = previousLauncher })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/validate":
			json.NewEncoder(w).Encode(map[string]any{"client_id": "client", "user_id": "user", "scopes": []string{scopeFollows}})
		case "/device":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if got := r.Form.Get("scopes"); got != scopeFollows+" "+scopeSubscriptions {
				t.Errorf("requested upgrade scopes = %q", got)
			}
			json.NewEncoder(w).Encode(map[string]any{"device_code": "device", "user_code": "code", "verification_uri": "https://example.test", "expires_in": 60, "interval": 1})
		case "/token":
			t.Error("cancelled upgrade should not poll for a token")
			http.Error(w, "unexpected token request", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	old := Credentials{AccessToken: "old-access", RefreshToken: "old-refresh"}
	store := &memoryCredentialStore{credentials: &old}
	ctx, cancel := context.WithCancel(context.Background())
	oauth := &OAuth{
		ClientID:              "client",
		Store:                 store,
		Subscriptions:         true,
		BaseURL:               server.URL,
		HTTP:                  server.Client(),
		OnDeviceAuthorization: func(string, string) { cancel() },
	}
	defer cancel()
	if _, err := oauth.Token(ctx); err == nil {
		t.Fatal("expected cancelled scope upgrade to fail")
	}
	if store.credentials == nil || *store.credentials != old || store.saveCount != 0 {
		t.Fatalf("saved credentials changed after cancelled upgrade: %+v saves=%d", store.credentials, store.saveCount)
	}
	if oauth.credentials == nil || *oauth.credentials != old {
		t.Fatalf("in-memory credentials changed after cancelled upgrade: %+v", oauth.credentials)
	}
}

func TestOAuthSubscriptionUpgradeSavesOnlyValidatedGrant(t *testing.T) {
	previousLauncher := browserLauncher
	browserLauncher = func(string) error { return nil }
	t.Cleanup(func() { browserLauncher = previousLauncher })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/validate":
			scopes := []string{scopeFollows}
			if r.Header.Get("Authorization") == "OAuth new-access" {
				scopes = append(scopes, scopeSubscriptions)
			}
			json.NewEncoder(w).Encode(map[string]any{"client_id": "client", "user_id": "user", "scopes": scopes})
		case "/device":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if got := r.Form.Get("scopes"); got != scopeFollows+" "+scopeSubscriptions {
				t.Errorf("requested upgrade scopes = %q", got)
			}
			json.NewEncoder(w).Encode(map[string]any{"device_code": "device", "user_code": "code", "verification_uri": "https://example.test", "expires_in": 60, "interval": 1})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if got := r.Form.Get("scopes"); got != scopeFollows+" "+scopeSubscriptions {
				t.Errorf("polled token scopes = %q", got)
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "new-access", "refresh_token": "new-refresh"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	old := Credentials{AccessToken: "old-access", RefreshToken: "old-refresh"}
	store := &memoryCredentialStore{credentials: &old}
	oauth := &OAuth{ClientID: "client", Store: store, Subscriptions: true, BaseURL: server.URL, HTTP: server.Client()}
	token, err := oauth.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "new-access" || store.saveCount != 1 {
		t.Fatalf("token = %q, saved credentials = %+v, saves = %d", token, store.credentials, store.saveCount)
	}
	want := Credentials{AccessToken: "new-access", RefreshToken: "new-refresh"}
	if *store.credentials != want {
		t.Fatalf("saved credentials = %+v, want %+v", store.credentials, want)
	}
}

func TestOAuthExpiredTokenRefreshThenUpgradesSubscriptionScope(t *testing.T) {
	previousLauncher := browserLauncher
	browserLauncher = func(string) error { return nil }
	t.Cleanup(func() { browserLauncher = previousLauncher })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/validate":
			switch r.Header.Get("Authorization") {
			case "OAuth old-access":
				w.WriteHeader(http.StatusUnauthorized)
			case "OAuth refreshed-access":
				json.NewEncoder(w).Encode(map[string]any{"client_id": "client", "user_id": "user", "scopes": []string{scopeFollows}})
			case "OAuth upgraded-access":
				json.NewEncoder(w).Encode(map[string]any{"client_id": "client", "user_id": "user", "scopes": []string{scopeFollows, scopeSubscriptions}})
			default:
				t.Errorf("unexpected validation authorization: %q", r.Header.Get("Authorization"))
				http.Error(w, "unexpected token", http.StatusUnauthorized)
			}
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("grant_type") == "refresh_token" {
				json.NewEncoder(w).Encode(map[string]any{"access_token": "refreshed-access", "refresh_token": "refreshed-refresh"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "upgraded-access", "refresh_token": "upgraded-refresh"})
		case "/device":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if got := r.Form.Get("scopes"); got != scopeFollows+" "+scopeSubscriptions {
				t.Errorf("requested upgrade scopes = %q", got)
			}
			json.NewEncoder(w).Encode(map[string]any{"device_code": "device", "user_code": "code", "verification_uri": "https://example.test", "expires_in": 60, "interval": 1})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	old := Credentials{AccessToken: "old-access", RefreshToken: "old-refresh"}
	store := &memoryCredentialStore{credentials: &old}
	oauth := &OAuth{
		ClientID:              "client",
		Store:                 store,
		Subscriptions:         true,
		BaseURL:               server.URL,
		HTTP:                  server.Client(),
		OnDeviceAuthorization: func(string, string) {},
	}
	token, err := oauth.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "upgraded-access" {
		t.Fatalf("token = %q, want upgraded access token", token)
	}
	want := Credentials{AccessToken: "upgraded-access", RefreshToken: "upgraded-refresh"}
	if store.credentials == nil || *store.credentials != want {
		t.Fatalf("saved credentials = %+v, want %+v", store.credentials, want)
	}
}

type memoryCredentialStore struct {
	credentials *Credentials
	saveCount   int
}

func (s *memoryCredentialStore) Load() (*Credentials, error) {
	if s.credentials == nil {
		return nil, nil
	}
	copy := *s.credentials
	return &copy, nil
}

func (s *memoryCredentialStore) Save(credentials Credentials) error {
	s.saveCount++
	s.credentials = &credentials
	return nil
}

func (s *memoryCredentialStore) Delete() error {
	s.credentials = nil
	return nil
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
