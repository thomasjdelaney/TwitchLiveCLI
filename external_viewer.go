package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultViewer  = "browser"
	defaultPlayer  = "mpv"
	defaultQuality = "middle"
)

type viewerOptions struct {
	Viewer     string
	Player     string
	Chat       bool
	Quality    string
	LowLatency bool
}

type launchPlan struct {
	url    string
	stream *exec.Cmd
	chat   *exec.Cmd
}

type launchDependencies struct {
	lookPath      func(string) (string, error)
	listQualities func(context.Context, string, string) ([]string, error)
}

var videoQualityPattern = regexp.MustCompile(`^(\d+)p(\d+)?$`)

func normalizeViewerOptions(options viewerOptions) viewerOptions {
	if options.Viewer == "" {
		options.Viewer = defaultViewer
	}
	if options.Player == "" {
		options.Player = defaultPlayer
	}
	if options.Quality == "" {
		options.Quality = defaultQuality
	}
	return options
}

func prepareLaunch(ctx context.Context, channel string, options viewerOptions) (launchPlan, error) {
	return prepareLaunchWith(ctx, channel, options, launchDependencies{
		lookPath:      exec.LookPath,
		listQualities: listStreamQualities,
	})
}

func prepareLaunchWith(ctx context.Context, channel string, options viewerOptions, deps launchDependencies) (launchPlan, error) {
	options = normalizeViewerOptions(options)
	plan := launchPlan{url: "https://www.twitch.tv/" + channel}
	if options.Chat {
		chatPath, err := deps.lookPath("chatterino")
		if err != nil {
			return launchPlan{}, fmt.Errorf("Chatterino is required when chat is enabled; install it or add it to PATH: %w", err)
		}
		plan.chat = exec.Command(chatPath, "-c", channel)
	}
	if options.Viewer == "browser" {
		return plan, nil
	}

	streamlinkPath, err := deps.lookPath("streamlink")
	if err != nil {
		return launchPlan{}, fmt.Errorf("Streamlink is required for viewer=streamlink; install it or add it to PATH: %w", err)
	}
	if _, err := deps.lookPath(options.Player); err != nil {
		return launchPlan{}, fmt.Errorf("%s is required as the Streamlink player; install it or add it to PATH: %w", options.Player, err)
	}

	quality := options.Quality
	if quality == "middle" {
		names, err := deps.listQualities(ctx, streamlinkPath, plan.url)
		if err != nil {
			return launchPlan{}, err
		}
		quality, err = middleVideoQuality(names)
		if err != nil {
			return launchPlan{}, err
		}
	}
	args := buildStreamlinkArgs(plan.url, quality, options.Player, options.LowLatency)
	plan.stream = exec.Command(streamlinkPath, args...)
	return plan, nil
}

func listStreamQualities(ctx context.Context, streamlinkPath, streamURL string) ([]string, error) {
	cmd := exec.CommandContext(ctx, streamlinkPath, "--json", "--loglevel", "none", streamURL)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("list Streamlink qualities: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("list Streamlink qualities: %w", err)
	}
	names, err := parseStreamlinkQualityJSON(output)
	if err != nil {
		return nil, err
	}
	return names, nil
}

func parseStreamlinkQualityJSON(data []byte) ([]string, error) {
	var response struct {
		Streams map[string]json.RawMessage `json:"streams"`
		Error   string                     `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("decode Streamlink quality list: %w", err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("Streamlink could not list qualities: %s", response.Error)
	}
	if response.Streams == nil {
		return nil, errors.New("Streamlink returned JSON without a stream list")
	}
	names := make([]string, 0, len(response.Streams))
	for name := range response.Streams {
		names = append(names, name)
	}
	return names, nil
}

func middleVideoQuality(names []string) (string, error) {
	type candidate struct {
		name       string
		resolution int
		frameRate  int
	}
	candidates := make([]candidate, 0, len(names))
	for _, name := range names {
		match := videoQualityPattern.FindStringSubmatch(name)
		if match == nil {
			continue
		}
		resolution, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		frameRate := 0
		if match[2] != "" {
			frameRate, err = strconv.Atoi(match[2])
			if err != nil {
				continue
			}
		}
		candidates = append(candidates, candidate{name: name, resolution: resolution, frameRate: frameRate})
	}
	if len(candidates) == 0 {
		if len(names) == 0 {
			return "", errors.New("Streamlink found no available qualities")
		}
		return "worst", nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].resolution != candidates[j].resolution {
			return candidates[i].resolution < candidates[j].resolution
		}
		if candidates[i].frameRate != candidates[j].frameRate {
			return candidates[i].frameRate < candidates[j].frameRate
		}
		return candidates[i].name < candidates[j].name
	})
	return candidates[(len(candidates)-1)/2].name, nil
}

func buildStreamlinkArgs(streamURL, quality, player string, lowLatency bool) []string {
	args := []string{"--player", player}
	if lowLatency {
		args = append(args, "--twitch-low-latency")
	}
	return append(args, streamURL, quality)
}

func startChat(plan launchPlan) error {
	if plan.chat == nil {
		return nil
	}
	if err := plan.chat.Start(); err != nil {
		return fmt.Errorf("start Chatterino: %w", err)
	}
	go func() { _ = plan.chat.Wait() }()
	return nil
}

func launchChannel(ctx context.Context, channel string, options viewerOptions) error {
	options = normalizeViewerOptions(options)
	plan, err := prepareLaunch(ctx, channel, options)
	if err != nil {
		return err
	}
	if err := startChat(plan); err != nil {
		return err
	}
	if plan.stream == nil {
		return openBrowser(plan.url)
	}
	plan.stream.Stdin = os.Stdin
	plan.stream.Stdout = os.Stdout
	plan.stream.Stderr = os.Stderr
	if err := plan.stream.Run(); err != nil {
		return fmt.Errorf("run Streamlink: %w", err)
	}
	return nil
}
