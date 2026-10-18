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
	"time"
)

const defaultClientID = "f0klcbai71dbp4g57py0wepdskarcs"
const defaultLiveModeInterval = time.Minute

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "twitch-live-cli:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("twitch-live-cli", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { printUsage(fs.Output()) }
	clientID := fs.String("client-id", envOr("TWITCH_CLIENT_ID", defaultClientID), "Twitch application client ID")
	jsonOutput := fs.Bool("json", false, "write live channels as JSON")
	tableOutput := fs.Bool("table", false, "show live channels in a compact table")
	liveMode := fs.Duration("live-mode", 0, "update the live channels screen periodically (defaults to 1m)")
	openLogin := fs.String("open", "", "open a live channel by login name in the browser")
	noStore := fs.Bool("no-store", false, "keep credentials only for this process")
	logout := fs.Bool("logout", false, "remove saved Twitch credentials")
	if err := fs.Parse(withDefaultLiveModeInterval(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if *jsonOutput && *tableOutput {
		return errors.New("--json and --table cannot be used together")
	}
	if *clientID == "" {
		return errors.New("client ID is empty; set --client-id or TWITCH_CLIENT_ID")
	}
	if *liveMode < 0 {
		return errors.New("--live-mode duration must be positive")
	}
	store := &CredentialStore{Disabled: *noStore}
	if *logout {
		if *noStore {
			return errors.New("--logout cannot be combined with --no-store")
		}
		if err := store.Delete(); err != nil {
			return fmt.Errorf("remove saved credentials: %w", err)
		}
		fmt.Println("Saved Twitch credentials removed.")
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	auth := &OAuth{ClientID: *clientID, Store: store}
	api := &Twitch{ClientID: *clientID, Auth: auth}
	if *liveMode > 0 && !*jsonOutput && *openLogin == "" {
		return runLiveModeTUI(ctx, api, *liveMode, *tableOutput)
	}
	for {
		streams, err := api.LiveFollowed(ctx)
		if err != nil {
			return err
		}
		if *openLogin != "" {
			var found *Stream
			for i := range streams {
				if strings.EqualFold(streams[i].Login, *openLogin) {
					found = &streams[i]
					break
				}
			}
			if found == nil {
				return fmt.Errorf("%q is not currently live among your followed channels", *openLogin)
			}
			return openBrowser("https://www.twitch.tv/" + found.Login)
		}
		if *jsonOutput {
			if err := json.NewEncoder(os.Stdout).Encode(streams); err != nil {
				return fmt.Errorf("write JSON: %w", err)
			}
		} else {
			render(streams, *tableOutput)
		}
		if *liveMode <= 0 {
			return nil
		}
		timer := time.NewTimer(*liveMode)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: twitch-live-cli [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fmt.Fprintf(w, "  %-24s %s\n", "--help", "Show this help message.")
	fmt.Fprintf(w, "  %-24s %s\n", "--json", "Write live channels as JSON.")
	fmt.Fprintf(w, "  %-24s %s\n", "--table", "Show live channels in a compact table.")
	fmt.Fprintf(w, "  %-24s %s\n", "--live-mode [DURATION]", "Update periodically (default 1m; for example, 2m). Press r to refresh or q to quit.")
	fmt.Fprintf(w, "  %-24s %s\n", "--open LOGIN", "Open a live followed channel in the browser.")
	fmt.Fprintf(w, "  %-24s %s\n", "--no-store", "Keep credentials only for this process.")
	fmt.Fprintf(w, "  %-24s %s\n", "--logout", "Remove saved Twitch credentials.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Developer options:")
	fmt.Fprintf(w, "  %-24s %s\n", "--client-id ID", "Use a different Twitch application client ID (usually unnecessary).")
	fmt.Fprintf(w, "  %-24s %s\n", "", "You can also set TWITCH_CLIENT_ID.")
}

func withDefaultLiveModeInterval(args []string) []string {
	result := make([]string, 0, len(args)+1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg != "--live-mode" {
			result = append(result, arg)
			continue
		}

		if i+1 < len(args) {
			next := args[i+1]
			if _, err := time.ParseDuration(next); err == nil || !strings.HasPrefix(next, "-") {
				result = append(result, arg, next)
				i++
				continue
			}
		}
		result = append(result, "--live-mode="+defaultLiveModeInterval.String())
	}
	return result
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
