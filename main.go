package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"
)

const defaultClientID = "f0klcbai71dbp4g57py0wepdskarcs"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "live-followed-cli:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("live-followed-cli", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	clientID := fs.String("client-id", envOr("TWITCH_CLIENT_ID", defaultClientID), "Twitch application client ID")
	jsonOutput := fs.Bool("json", false, "write live channels as JSON")
	watch := fs.Duration("watch", 0, "refresh periodically (for example: 2m)")
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
	if *clientID == "" {
		return errors.New("client ID is empty; set --client-id or TWITCH_CLIENT_ID")
	}
	if *watch < 0 {
		return errors.New("--watch duration must be positive")
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
			if *watch > 0 {
				fmt.Printf("\nLive followed channels · %s\n", time.Now().Format(time.RFC1123))
			}
			render(streams)
		}
		if *watch <= 0 {
			return nil
		}
		timer := time.NewTimer(*watch)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
