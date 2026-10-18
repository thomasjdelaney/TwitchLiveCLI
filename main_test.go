package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
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
