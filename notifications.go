package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gen2brain/beeep"
)

type desktopNotifier interface {
	Notify(title, message string) error
}

type beeepNotifier struct{}

func (beeepNotifier) Notify(title, message string) error { return beeep.Notify(title, message, "") }

func notificationBody(stream Stream, now time.Time) string {
	return strings.Join([]string{
		fmt.Sprintf("%s  %s", streamCardChannelName(stream), streamCardViewers(stream)),
		"  " + stream.Title,
		"  " + streamCardCategoryLine(stream, now),
	}, "\n")
}

func notificationsForRefresh(streams []Stream, previous map[string]bool, hasPrevious, notifyAtStartup bool, enabledIDs []string) ([]Stream, map[string]bool) {
	enabled := make(map[string]bool, len(enabledIDs))
	for _, id := range enabledIDs {
		enabled[id] = true
	}
	current := make(map[string]bool, len(streams))
	toNotify := make([]Stream, 0)
	for _, stream := range streams {
		current[stream.UserID] = true
		wasLive := previous[stream.UserID]
		if enabled[stream.UserID] && ((!hasPrevious && notifyAtStartup) || (hasPrevious && !wasLive)) {
			toNotify = append(toNotify, stream)
		}
	}
	return toNotify, current
}
