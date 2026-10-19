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
	ClientID          string
	Auth              *OAuth
	Subscriptions     bool
	subscriptionCache map[string]SubscriptionInfo
	HTTP              *http.Client
	BaseURL           string
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
	ID           string            `json:"id"`
	UserID       string            `json:"user_id"`
	Login        string            `json:"user_login"`
	Name         string            `json:"user_name"`
	Game         string            `json:"game_name"`
	Title        string            `json:"title"`
	Viewers      int               `json:"viewer_count"`
	StartedAt    time.Time         `json:"started_at"`
	Subscription *SubscriptionInfo `json:"subscription,omitempty"`
}

type SubscriptionInfo struct {
	Subscribed bool `json:"subscribed"`
	Tier       int  `json:"tier,omitempty"`
}

type twitchAPIError struct {
	Status  int
	Message string
}

func (e *twitchAPIError) Error() string {
	return fmt.Sprintf("Twitch API returned HTTP %d: %s", e.Status, e.Message)
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
	return t.LiveFollowedSorted(ctx, defaultSortBy, defaultSortOrder)
}

func (t *Twitch) LiveFollowedSorted(ctx context.Context, sortBy, sortOrder string) ([]Stream, error) {
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
	if t.Subscriptions {
		if err := t.enrichSubscriptions(ctx, userID, streams); err != nil {
			return nil, err
		}
	}
	sortStreams(streams, sortBy, sortOrder)
	return streams, nil
}

func (t *Twitch) enrichSubscriptions(ctx context.Context, viewerID string, streams []Stream) error {
	if t.subscriptionCache == nil {
		t.subscriptionCache = make(map[string]SubscriptionInfo)
	}
	for i := range streams {
		broadcasterID := streams[i].UserID
		if info, ok := t.subscriptionCache[broadcasterID]; ok {
			cached := info
			streams[i].Subscription = &cached
			continue
		}
		info, err := t.subscription(ctx, viewerID, broadcasterID)
		if err != nil {
			return fmt.Errorf("check subscription for %s: %w", streams[i].Login, err)
		}
		t.subscriptionCache[broadcasterID] = info
		cached := info
		streams[i].Subscription = &cached
	}
	return nil
}

func (t *Twitch) subscription(ctx context.Context, viewerID, broadcasterID string) (SubscriptionInfo, error) {
	values := url.Values{"user_id": {viewerID}, "broadcaster_id": {broadcasterID}}
	var response struct {
		Data []struct {
			Tier string `json:"tier"`
		} `json:"data"`
	}
	err := t.get(ctx, "/subscriptions/user", values, &response)
	if err != nil {
		var apiErr *twitchAPIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return SubscriptionInfo{}, nil
		}
		return SubscriptionInfo{}, err
	}
	if len(response.Data) == 0 {
		return SubscriptionInfo{}, nil
	}
	tier, err := strconv.Atoi(response.Data[0].Tier)
	if err != nil {
		return SubscriptionInfo{}, fmt.Errorf("decode subscription tier %q: %w", response.Data[0].Tier, err)
	}
	return SubscriptionInfo{Subscribed: true, Tier: tier}, nil
}

func sortStreams(streams []Stream, sortBy, sortOrder string) {
	sort.SliceStable(streams, func(i, j int) bool {
		left, right := streams[i], streams[j]
		if sortBy == "live_duration" {
			leftHasStart, rightHasStart := !left.StartedAt.IsZero(), !right.StartedAt.IsZero()
			if leftHasStart != rightHasStart {
				return leftHasStart
			}
			if leftHasStart && !left.StartedAt.Equal(right.StartedAt) {
				if sortOrder == "desc" {
					return left.StartedAt.Before(right.StartedAt)
				}
				return left.StartedAt.After(right.StartedAt)
			}
		} else {
			comparison := compareStreams(left, right, sortBy)
			if comparison != 0 {
				if sortOrder == "desc" {
					return comparison > 0
				}
				return comparison < 0
			}
		}

		leftLogin, rightLogin := strings.ToLower(left.Login), strings.ToLower(right.Login)
		if leftLogin == rightLogin {
			return left.Name < right.Name
		}
		return leftLogin < rightLogin
	})
}

func compareStreams(left, right Stream, sortBy string) int {
	var leftValue, rightValue string
	switch sortBy {
	case "channel":
		leftValue, rightValue = left.Name, right.Name
		if leftValue == "" {
			leftValue = left.Login
		}
		if rightValue == "" {
			rightValue = right.Login
		}
	case "title":
		leftValue, rightValue = left.Title, right.Title
	case "category":
		leftValue, rightValue = left.Game, right.Game
	case "subscribed":
		leftSubscribed := left.Subscription != nil && left.Subscription.Subscribed
		rightSubscribed := right.Subscription != nil && right.Subscription.Subscribed
		if leftSubscribed == rightSubscribed {
			return 0
		}
		if leftSubscribed {
			return 1
		}
		return -1
	case "sub_tier":
		leftTier, rightTier := 0, 0
		if left.Subscription != nil {
			leftTier = left.Subscription.Tier
		}
		if right.Subscription != nil {
			rightTier = right.Subscription.Tier
		}
		if leftTier < rightTier {
			return -1
		}
		if leftTier > rightTier {
			return 1
		}
		return 0
	case "viewers":
		if left.Viewers < right.Viewers {
			return -1
		}
		if left.Viewers > right.Viewers {
			return 1
		}
		return 0
	default:
		return 0
	}
	return strings.Compare(strings.ToLower(leftValue), strings.ToLower(rightValue))
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
			return &twitchAPIError{Status: resp.StatusCode, Message: apiErr.Message}
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
