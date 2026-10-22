package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/charmbracelet/x/term"
)

const defaultClientID = "sz30kdfjwx1vpdn9f1k00r6brg5zbg"

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "twitch-live-cli:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("twitch-live-cli", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	options := registerFlags(fs)
	fs.Usage = func() { printUsage(fs.Output()) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if *options.showVersion && *options.update {
		return errors.New("--version and --update cannot be used together")
	}
	if *options.showVersion {
		fmt.Printf("twitch-live-cli %s\n", version)
		return nil
	}
	if *options.update {
		return updateCommand()
	}
	provided := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if *options.jsonOutput && provided["display"] {
		return errors.New("--json and --display cannot be used together")
	}
	if *options.openLogin != "" && (*options.once || *options.jsonOutput || provided["display"]) {
		return errors.New("--open cannot be combined with --once, --json, or --display")
	}
	if *options.clientID == "" {
		return errors.New("client ID is empty; set --client-id or TWITCH_CLIENT_ID")
	}
	store := &CredentialStore{Disabled: *options.noStore}
	if *options.logout {
		if *options.noStore {
			return errors.New("--logout cannot be combined with --no-store")
		}
		if err := store.Delete(); err != nil {
			return fmt.Errorf("remove saved credentials: %w", err)
		}
		fmt.Println("Saved Twitch credentials removed.")
		return nil
	}
	basePreferences, err := loadPreferences(*options.configPath)
	if err != nil {
		return err
	}
	resolvedPreferences, err := resolvePreferences(basePreferences, preferenceOverrides{
		Display:         override[string]{Value: *options.display, Set: provided["display"]},
		SortBy:          override[string]{Value: *options.sortBy, Set: provided["sort-by"]},
		SortOrder:       override[string]{Value: *options.sortOrder, Set: provided["sort-order"]},
		RefreshInterval: override[string]{Value: *options.refreshInterval, Set: provided["refresh-interval"]},
		Subscriptions:   override[bool]{Value: *options.subscriptions, Set: provided["subscriptions"]},
		Viewer:          override[string]{Value: *options.viewer, Set: provided["viewer"]},
		Player:          override[string]{Value: *options.player, Set: provided["player"]},
		Chat:            override[bool]{Value: *options.chat, Set: provided["chat"]},
		Quality:         override[string]{Value: *options.quality, Set: provided["quality"]},
		LowLatency:      override[bool]{Value: *options.lowLatency, Set: provided["low-latency"]},
	})
	if err != nil {
		return err
	}
	var authLogFile *os.File
	if *options.authLog != "" {
		authLogFile, err = os.OpenFile(*options.authLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("open auth log: %w", err)
		}
		defer authLogFile.Close()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	auth := &OAuth{ClientID: *options.clientID, Store: store, Subscriptions: resolvedPreferences.Subscriptions}
	willRunLiveMode := shouldRunLiveMode(*options.once, *options.jsonOutput, *options.openLogin, interactiveTerminal())
	auth.LogWriter = authLogDestination(!willRunLiveMode && *options.openLogin == "", authLogFile)
	api := &Twitch{ClientID: *options.clientID, Auth: auth, Subscriptions: resolvedPreferences.Subscriptions}
	viewerOptions := viewerOptionsFromPreferences(resolvedPreferences)
	if willRunLiveMode {
		return runLiveModeTUI(ctx, api, resolvedPreferences.RefreshInterval, resolvedPreferences.Display, resolvedPreferences.SortBy, resolvedPreferences.SortOrder, viewerOptions)
	}
	streams, err := api.LiveFollowedSorted(ctx, resolvedPreferences.SortBy, resolvedPreferences.SortOrder)
	if err != nil {
		return err
	}
	if *options.openLogin != "" {
		var found *Stream
		for i := range streams {
			if strings.EqualFold(streams[i].Login, *options.openLogin) {
				found = &streams[i]
				break
			}
		}
		if found == nil {
			return fmt.Errorf("%q is not currently live among your followed channels", *options.openLogin)
		}
		return launchChannel(ctx, found.Login, viewerOptions)
	}
	if *options.jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(streams); err != nil {
			return fmt.Errorf("write JSON: %w", err)
		}
	} else {
		render(streams, resolvedPreferences.Display == "table")
	}
	return nil
}

func authLogDestination(snapshotOutput bool, file io.Writer) io.Writer {
	if file != nil {
		return file
	}
	if snapshotOutput {
		return io.Discard
	}
	return nil
}

func shouldRunLiveMode(once, jsonOutput bool, openLogin string, interactive bool) bool {
	return interactive && !once && !jsonOutput && openLogin == ""
}

func interactiveTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

type cliFlags struct {
	clientID, display, sortBy, sortOrder, refreshInterval *string
	viewer, player, quality, configPath, openLogin        *string
	jsonOutput, subscriptions, chat, lowLatency           *bool
	once, noStore, logout                                 *bool
	showVersion                                           *bool
	update                                                *bool
	authLog                                               *string
}

func registerFlags(fs *flag.FlagSet) cliFlags {
	return cliFlags{
		clientID:        fs.String("client-id", envOr("TWITCH_CLIENT_ID", defaultClientID), "Twitch application client ID (usually unnecessary)"),
		jsonOutput:      fs.Bool("json", false, "write live channels as JSON"),
		display:         fs.String("display", "", "display style: table or card (default: table)"),
		sortBy:          fs.String("sort-by", "", "sort by viewers, channel, title, category, live_duration, subscribed, or sub_tier"),
		sortOrder:       fs.String("sort-order", "", "sort order: asc or desc"),
		refreshInterval: fs.String("refresh-interval", "", "live screen refresh interval (default 1m; minimum 2s; e.g. 2m)"),
		subscriptions:   fs.Bool("subscriptions", false, "include subscription status and tier; requests extra Twitch permission"),
		viewer:          fs.String("viewer", "", "viewer: browser (default) or streamlink"),
		player:          fs.String("player", "", "Streamlink player: mpv (default) or vlc"),
		chat:            fs.Bool("chat", false, "open Chatterino alongside the stream"),
		quality:         fs.String("quality", "", "Streamlink quality (default: middle available video quality)"),
		lowLatency:      fs.Bool("low-latency", false, "enable Streamlink Twitch low-latency mode"),
		once:            fs.Bool("once", false, "show one snapshot instead of the live screen"),
		configPath:      fs.String("config", "", "read preferences from this TOML file"),
		openLogin:       fs.String("open", "", "open a live channel by login name with the selected viewer"),
		noStore:         fs.Bool("no-store", false, "keep credentials only for this process"),
		logout:          fs.Bool("logout", false, "remove saved Twitch credentials"),
		showVersion:     fs.Bool("version", false, "print version and exit"),
		update:          fs.Bool("update", false, "download and install the latest release"),
		authLog:         fs.String("auth-log", "", "write auth diagnostics to this file (developer option)"),
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: twitch-live-cli [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs := flag.NewFlagSet("twitch-live-cli", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	registerFlags(fs)
	fmt.Fprintf(w, "  %-32s %s\n", "--help", "Show this help message.")
	fs.VisitAll(func(f *flag.Flag) {
		if f.Name == "client-id" || f.Name == "auth-log" {
			return
		}
		placeholder := map[string]string{
			"display": " MODE", "sort-by": " FIELD", "sort-order": " ORDER", "refresh-interval": " DURATION",
			"viewer": " MODE", "player": " NAME", "quality": " NAME", "config": " PATH", "open": " LOGIN",
		}[f.Name]
		fmt.Fprintf(w, "  %-32s %s\n", "--"+f.Name+placeholder, f.Usage)
	})
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Developer options:")
	clientID := fs.Lookup("client-id")
	fmt.Fprintf(w, "  %-32s %s\n", "--client-id ID", clientID.Usage)
	fmt.Fprintf(w, "  %-32s %s\n", "", "You can also set TWITCH_CLIENT_ID.")
	authLog := fs.Lookup("auth-log")
	fmt.Fprintf(w, "  %-32s %s\n", "--auth-log PATH", authLog.Usage)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
