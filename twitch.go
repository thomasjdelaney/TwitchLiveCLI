package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const helixBase = "https://api.twitch.tv/helix"

type Twitch struct {
	ClientID string
	Auth     *OAuth
	HTTP     *http.Client
	BaseURL  string
}
type followedResponse struct {
	Data       []Followed `json:"data"`
	Pagination struct {
		Cursor string `json:"cursor"`
	} `json:"pagination"`
}
type Followed struct {
	ID    string `json:"broadcaster_id"`
	Login string `json:"broadcaster_login"`
	Name  string `json:"broadcaster_name"`
}
type streamResponse struct {
	Data []Stream `json:"data"`
}
type Stream struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Login     string    `json:"user_login"`
	Name      string    `json:"user_name"`
	Game      string    `json:"game_name"`
	Title     string    `json:"title"`
	Viewers   int       `json:"viewer_count"`
	StartedAt time.Time `json:"started_at"`
}

func (t *Twitch) client() *http.Client {
	if t.HTTP != nil {
		return t.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}
func (t *Twitch) baseURL() string {
	if t.BaseURL != "" {
		return strings.TrimRight(t.BaseURL, "/")
	}
	return helixBase
}

func (t *Twitch) LiveFollowed(ctx context.Context) ([]Stream, error) {
	userID, err := t.Auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	follows, err := t.followed(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(follows) == 0 {
		return []Stream{}, nil
	}
	streams := make([]Stream, 0)
	for start := 0; start < len(follows); start += 100 {
		end := start + 100
		if end > len(follows) {
			end = len(follows)
		}
		values := url.Values{}
		for _, follow := range follows[start:end] {
			values.Add("user_id", follow.ID)
		}
		var response streamResponse
		if err := t.get(ctx, "/streams", values, &response); err != nil {
			return nil, err
		}
		streams = append(streams, response.Data...)
	}
	sort.Slice(streams, func(i, j int) bool {
		if streams[i].Viewers == streams[j].Viewers {
			return strings.ToLower(streams[i].Login) < strings.ToLower(streams[j].Login)
		}
		return streams[i].Viewers > streams[j].Viewers
	})
	return streams, nil
}

func (o *OAuth) UserID(ctx context.Context) (string, error) {
	if _, err := o.Token(ctx); err != nil {
		return "", err
	}
	// A successful validation records the user ID as part of the OAuth session.
	return o.userID, nil
}

func (t *Twitch) followed(ctx context.Context, userID string) ([]Followed, error) {
	follows := make([]Followed, 0)
	var cursor string
	for {
		values := url.Values{"user_id": {userID}, "first": {"100"}}
		if cursor != "" {
			values.Set("after", cursor)
		}
		var response followedResponse
		if err := t.get(ctx, "/channels/followed", values, &response); err != nil {
			return nil, fmt.Errorf("get followed channels: %w", err)
		}
		follows = append(follows, response.Data...)
		if response.Pagination.Cursor == "" {
			break
		}
		if response.Pagination.Cursor == cursor {
			return nil, errors.New("Twitch returned a repeated pagination cursor")
		}
		cursor = response.Pagination.Cursor
	}
	return follows, nil
}

func (t *Twitch) get(ctx context.Context, path string, query url.Values, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := t.Auth.Token(ctx)
		if err != nil {
			return err
		}
		endpoint := t.baseURL() + path
		if len(query) > 0 {
			endpoint += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Client-Id", t.ClientID)
		resp, err := t.client().Do(req)
		if err != nil {
			return fmt.Errorf("Twitch API request failed: %w", err)
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if err := t.Auth.RefreshAfterUnauthorized(ctx); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			reset := rateLimitReset(resp.Header)
			resp.Body.Close()
			if reset.IsZero() {
				return errors.New("Twitch rate limit reached; try again shortly")
			}
			return fmt.Errorf("Twitch rate limit reached; try again after %s", reset.Local().Format(time.Kitchen))
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			var apiErr struct {
				Message string `json:"message"`
			}
			_ = json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&apiErr)
			resp.Body.Close()
			if apiErr.Message == "" {
				apiErr.Message = http.StatusText(resp.StatusCode)
			}
			return fmt.Errorf("Twitch API returned HTTP %d: %s", resp.StatusCode, apiErr.Message)
		}
		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("decode Twitch API response: %w", err)
		}
		return nil
	}
	return errors.New("Twitch authorization failed after refresh")
}

func rateLimitReset(h http.Header) time.Time {
	value := h.Get("Ratelimit-Reset")
	if value == "" {
		value = h.Get("Retry-After")
	}
	if value == "" {
		return time.Time{}
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}
	if h.Get("Ratelimit-Reset") == "" {
		return time.Now().Add(time.Duration(seconds) * time.Second)
	}
	return time.Unix(seconds, 0)
}
