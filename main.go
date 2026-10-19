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

const defaultClientID = "f0klcbai71dbp4g57py0wepdskarcs"

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
	display := fs.String("display", "", "display style: table or card")
	sortBy := fs.String("sort-by", "", "sort by: viewers, channel, title, category, or live_duration")
	sortOrder := fs.String("sort-order", "", "sort order: asc or desc")
	refreshInterval := fs.String("refresh-interval", "", "live screen refresh interval (minimum 2s; for example, 2m)")
	once := fs.Bool("once", false, "show one snapshot instead of the live screen")
	configPath := fs.String("config", "", "read preferences from this TOML file")
	openLogin := fs.String("open", "", "open a live channel by login name in the browser")
	noStore := fs.Bool("no-store", false, "keep credentials only for this process")
	logout := fs.Bool("logout", false, "remove saved Twitch credentials")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	provided := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if *jsonOutput && provided["display"] {
		return errors.New("--json and --display cannot be used together")
	}
	if *clientID == "" {
		return errors.New("client ID is empty; set --client-id or TWITCH_CLIENT_ID")
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

	basePreferences, err := loadPreferences(*configPath)
	if err != nil {
		return err
	}
	resolvedPreferences, err := resolvePreferences(basePreferences, preferenceOverrides{
		Display:         *display,
		SortBy:          *sortBy,
		SortOrder:       *sortOrder,
		RefreshInterval: *refreshInterval,
		DisplaySet:      provided["display"],
		SortBySet:       provided["sort-by"],
		SortOrderSet:    provided["sort-order"],
		RefreshSet:      provided["refresh-interval"],
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	auth := &OAuth{ClientID: *clientID, Store: store}
	api := &Twitch{ClientID: *clientID, Auth: auth}
	if shouldRunLiveMode(*once, *jsonOutput, *openLogin, interactiveTerminal()) {
		return runLiveModeTUI(ctx, api, resolvedPreferences.RefreshInterval, resolvedPreferences.Display, resolvedPreferences.SortBy, resolvedPreferences.SortOrder)
	}
	streams, err := api.LiveFollowedSorted(ctx, resolvedPreferences.SortBy, resolvedPreferences.SortOrder)
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
		render(streams, resolvedPreferences.Display == "table")
	}
	return nil
}

func shouldRunLiveMode(once, jsonOutput bool, openLogin string, interactive bool) bool {
	return interactive && !once && !jsonOutput && openLogin == ""
}

func interactiveTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: twitch-live-cli [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fmt.Fprintf(w, "  %-32s %s\n", "--help", "Show this help message.")
	fmt.Fprintf(w, "  %-32s %s\n", "--json", "Write live channels as JSON.")
	fmt.Fprintf(w, "  %-32s %s\n", "--display MODE", "Choose table or card display (default: table).")
	fmt.Fprintf(w, "  %-32s %s\n", "--sort-by FIELD", "Sort by viewers, channel, title, category, or live_duration.")
	fmt.Fprintf(w, "  %-32s %s\n", "--sort-order ORDER", "Sort ascending or descending.")
	fmt.Fprintf(w, "  %-32s %s\n", "--refresh-interval DURATION", "Set live screen refresh interval (default 1m, minimum 2s).")
	fmt.Fprintf(w, "  %-32s %s\n", "--once", "Show one snapshot instead of the default live screen.")
	fmt.Fprintf(w, "  %-32s %s\n", "--config PATH", "Read preferences from a TOML file.")
	fmt.Fprintf(w, "  %-32s %s\n", "--open LOGIN", "Open a live followed channel in the browser.")
	fmt.Fprintf(w, "  %-32s %s\n", "--no-store", "Keep credentials only for this process.")
	fmt.Fprintf(w, "  %-32s %s\n", "--logout", "Remove saved Twitch credentials.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Developer options:")
	fmt.Fprintf(w, "  %-32s %s\n", "--client-id ID", "Use a different Twitch application client ID (usually unnecessary).")
	fmt.Fprintf(w, "  %-32s %s\n", "", "You can also set TWITCH_CLIENT_ID.")
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
