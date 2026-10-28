package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingNotifier struct {
	titles []string
	bodies []string
	err    error
}

func TestNotificationsForRefresh(t *testing.T) {
	first := []Stream{{UserID: "enabled", Login: "sample"}, {UserID: "other", Login: "other"}}
	got, state := notificationsForRefresh(first, nil, false, false, []string{"enabled"})
	if len(got) != 0 || !state["enabled"] {
		t.Fatalf("startup disabled: notifications=%v state=%v", got, state)
	}
	got, state = notificationsForRefresh(first, nil, false, true, []string{"enabled"})
	if len(got) != 1 || got[0].UserID != "enabled" {
		t.Fatalf("startup enabled notifications = %v", got)
	}
	got, state = notificationsForRefresh(first, state, true, true, []string{"enabled"})
	if len(got) != 0 {
		t.Fatalf("repeated live refresh notifications = %v", got)
	}
	got, state = notificationsForRefresh(nil, state, true, true, []string{"enabled"})
	got, _ = notificationsForRefresh(first, state, true, true, []string{"enabled"})
	if len(got) != 1 {
		t.Fatalf("later offline-to-live notifications = %v", got)
	}
}

func (n *recordingNotifier) Notify(title, message string) error {
	n.titles = append(n.titles, title)
	n.bodies = append(n.bodies, message)
	return n.err
}

func TestNotificationCommandUsesNotifierAndSurfacesErrors(t *testing.T) {
	stream := Stream{UserID: "123", Name: "Channel", Login: "channel", Title: "Live now", Game: "Speedrunning", Viewers: 1234, StartedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), Subscription: &SubscriptionInfo{Subscribed: true, Tier: 1000}}
	notifier := &recordingNotifier{}
	model := liveModeModel{notifier: notifier}
	result := model.notify([]Stream{stream})()
	if msg, ok := result.(notificationResultMsg); !ok || msg.err != nil {
		t.Fatalf("notification result = %#v", result)
	}
	if len(notifier.titles) != 1 || !strings.HasPrefix(notifier.bodies[0], "Channel  1,234 viewers\n  Live now\n  Speedrunning  · live for ") || strings.Contains(notifier.bodies[0], "Subscription:") {
		t.Fatalf("sent notifications = %+v", notifier)
	}
	if got := notificationBody(stream, stream.StartedAt.Add(90*time.Minute)); got != "Channel  1,234 viewers\n  Live now\n  Speedrunning  · live for 1h 30m" {
		t.Fatalf("notification body = %q", got)
	}
	stream.Viewers = 1234
	stream.Name = ""
	stream.Login = "channel-login"
	stream.Game = ""
	stream.StartedAt = time.Time{}
	if got := notificationBody(stream, time.Now()); got != "channel-login  1,234 viewers\n  Live now\n  No category" {
		t.Fatalf("notification body fallback = %q", got)
	}

	wantErr := errors.New("desktop unavailable")
	notifier.err = wantErr
	result = model.notify([]Stream{stream})()
	if msg, ok := result.(notificationResultMsg); !ok || !errors.Is(msg.err, wantErr) {
		t.Fatalf("notification error result = %#v", result)
	}
	toNotify, seen := notificationsForRefresh([]Stream{stream}, nil, false, true, []string{stream.UserID})
	if len(toNotify) != 1 {
		t.Fatalf("initial live stream notifications = %v", toNotify)
	}
	next, _ := notificationsForRefresh([]Stream{stream}, seen, true, true, []string{stream.UserID})
	if len(next) != 0 {
		t.Fatalf("failed delivery was queued again while channel stayed live: %v", next)
	}
}
