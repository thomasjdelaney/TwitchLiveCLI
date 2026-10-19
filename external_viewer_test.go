package main

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestMiddleVideoQualitySelectsLowerMedian(t *testing.T) {
	got, err := middleVideoQuality([]string{"1080p60", "160p", "720p60", "360p", "480p", "audio_only", "best", "worst"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "480p" {
		t.Fatalf("middle quality = %q, want 480p", got)
	}
}

func TestMiddleVideoQualityUsesFrameRateAsTieBreaker(t *testing.T) {
	got, err := middleVideoQuality([]string{"720p60", "720p", "480p"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "720p" {
		t.Fatalf("middle quality = %q, want 720p", got)
	}
}

func TestMiddleVideoQualityFallsBackToWorstForUnrecognizedNames(t *testing.T) {
	got, err := middleVideoQuality([]string{"audio_only", "best", "source", "chunked"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "worst" {
		t.Fatalf("fallback quality = %q, want worst", got)
	}
}

func TestMiddleVideoQualityErrorsWhenNoStreamsExist(t *testing.T) {
	if _, err := middleVideoQuality(nil); err == nil {
		t.Fatal("expected no-stream error")
	}
}

func TestParseStreamlinkQualityJSON(t *testing.T) {
	got, err := parseStreamlinkQualityJSON([]byte(`{"plugin":"twitch","streams":{"audio_only":{},"720p60":{},"best":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"720p60", "audio_only", "best"}) {
		t.Fatalf("quality names = %v", got)
	}
}

func TestParseStreamlinkQualityJSONRejectsMissingStreamList(t *testing.T) {
	if _, err := parseStreamlinkQualityJSON([]byte(`{"plugin":"twitch"}`)); err == nil {
		t.Fatal("expected missing streams error")
	}
}

func TestBuildStreamlinkArgs(t *testing.T) {
	base := []string{"--player", "mpv", "https://www.twitch.tv/channel", "720p60"}
	if got := buildStreamlinkArgs("https://www.twitch.tv/channel", "720p60", "mpv", false); !reflect.DeepEqual(got, base) {
		t.Fatalf("Streamlink args = %v, want %v", got, base)
	}
	withLatency := []string{"--player", "vlc", "--twitch-low-latency", "https://www.twitch.tv/channel", "480p"}
	if got := buildStreamlinkArgs("https://www.twitch.tv/channel", "480p", "vlc", true); !reflect.DeepEqual(got, withLatency) {
		t.Fatalf("low-latency args = %v, want %v", got, withLatency)
	}
}

func TestPrepareLaunchExplicitQualitySkipsDiscoveryAndBuildsChatterinoArgs(t *testing.T) {
	lookPath := func(name string) (string, error) {
		switch name {
		case "streamlink":
			return "/tools/streamlink", nil
		case "mpv":
			return "/tools/mpv", nil
		case "chatterino":
			return "/tools/chatterino", nil
		default:
			return "", errors.New("not found")
		}
	}
	listCalled := false
	plan, err := prepareLaunchWith(context.Background(), "channel", viewerOptions{
		Viewer: "streamlink", Player: "mpv", Chat: true, Quality: "720p60",
	}, launchDependencies{
		lookPath: lookPath,
		listQualities: func(context.Context, string, string) ([]string, error) {
			listCalled = true
			return nil, errors.New("quality discovery should be skipped")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if listCalled {
		t.Fatal("explicit quality should bypass discovery")
	}
	if plan.stream == nil || !reflect.DeepEqual(plan.stream.Args, []string{"/tools/streamlink", "--player", "mpv", "https://www.twitch.tv/channel", "720p60"}) {
		t.Fatalf("streamlink command = %#v", plan.stream)
	}
	if plan.chat == nil || !reflect.DeepEqual(plan.chat.Args, []string{"/tools/chatterino", "-c", "channel"}) {
		t.Fatalf("Chatterino command = %#v", plan.chat)
	}
}

func TestPrepareLaunchMiddleQualityUsesDiscoveredNames(t *testing.T) {
	var listedURL string
	plan, err := prepareLaunchWith(context.Background(), "channel", viewerOptions{
		Viewer: "streamlink", Player: "vlc", Quality: "middle",
	}, launchDependencies{
		lookPath: func(name string) (string, error) { return "/tools/" + name, nil },
		listQualities: func(_ context.Context, path, streamURL string) ([]string, error) {
			if path != "/tools/streamlink" {
				t.Errorf("Streamlink path = %q", path)
			}
			listedURL = streamURL
			return []string{"160p", "360p", "720p60", "audio_only", "best"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if listedURL != "https://www.twitch.tv/channel" {
		t.Fatalf("quality discovery URL = %q", listedURL)
	}
	if got := plan.stream.Args[len(plan.stream.Args)-1]; got != "360p" {
		t.Fatalf("selected quality = %q, want 360p", got)
	}
}

func TestPrepareLaunchReportsMissingRuntimeDependency(t *testing.T) {
	_, err := prepareLaunchWith(context.Background(), "channel", viewerOptions{Viewer: "streamlink", Player: "mpv", Quality: "best"}, launchDependencies{
		lookPath: func(name string) (string, error) {
			if name == "streamlink" {
				return "", errors.New("not found")
			}
			return "/tools/" + name, nil
		},
	})
	if err == nil || !contains(err.Error(), "Streamlink is required") {
		t.Fatalf("missing dependency error = %v", err)
	}
}

func TestPrepareBrowserWithChatDoesNotRequireStreamlink(t *testing.T) {
	plan, err := prepareLaunchWith(context.Background(), "channel", viewerOptions{Viewer: "browser", Chat: true}, launchDependencies{
		lookPath: func(name string) (string, error) {
			if name == "chatterino" {
				return "/tools/chatterino", nil
			}
			return "", errors.New("not found")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.stream != nil || plan.chat == nil {
		t.Fatalf("browser/chat launch plan = %+v", plan)
	}
}

func contains(value, substring string) bool {
	return len(substring) == 0 || (len(value) >= len(substring) && strings.Contains(value, substring))
}
