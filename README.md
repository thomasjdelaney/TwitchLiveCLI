# TwitchLiveCLI

See which of your followed Twitch channels are live from your terminal. The command is `twitch-live-cli`.

## Get started

Run `twitch-live-cli` in an interactive terminal to open the live screen. It refreshes every minute by default. Use the arrow keys or `j`/`k` to select a channel, press Enter to open it in your browser, `r` to refresh, and `q` to quit. Home/End select the first or last channel; PgUp/PgDn scroll. Without an interactive terminal, the command prints one snapshot.

The first time you run it, follow the sign-in link and enter the code shown in the terminal. The CLI then lists live followed channels, sorted by viewer count. If none are live, it says so.

## Commands

| Command | What it does |
| --- | --- |
| `twitch-live-cli --once` | Print one snapshot, even in an interactive terminal. |
| `twitch-live-cli --json` | Print live channels as JSON. |
| `twitch-live-cli --display card` | Show channels as cards instead of the default table. |
| `twitch-live-cli --sort-by channel --sort-order asc` | Choose a sort field and direction. |
| `twitch-live-cli --subscriptions` | Include your subscription status and tier. |
| `twitch-live-cli --subscriptions --sort-by sub_tier` | Sort by subscription tier. |
| `twitch-live-cli --refresh-interval 2m` | Refresh the live screen every two minutes. Minimum: `2s`. |
| `twitch-live-cli --config PATH` | Read preferences from a TOML file. |
| `twitch-live-cli --open twitchdev` | Open a followed channel if it is live. |
| `twitch-live-cli --logout` | Remove saved Twitch credentials. |
| `twitch-live-cli --no-store` | Sign in without saving credentials. |
| `twitch-live-cli --help` | Show all options. |

Sort fields are `viewers`, `channel`, `title`, `category`, `live_duration`, `subscribed`, and `sub_tier`. The default is viewers descending; channel, title, and category default to ascending, and the other fields to descending. Sorting also applies to JSON output. `--json` cannot be combined with `--display`. `--open` takes a channel login, such as `twitchdev`.

Subscription details are off by default. Enabling them requests Twitch's `user:read:subscriptions` permission in addition to `user:read:follows`. If your saved authorization lacks the subscription permission, the CLI asks you to authorize it.

## Preferences

The optional `twitch-live-cli/config.toml` file is read from your operating system's user config directory. Use `--config PATH` to choose a different file. Example:

```toml
display = "card"
sort_by = "channel"
sort_order = "asc"
refresh_interval = "2m"
subscriptions = true
```

Use `table` or `card` for `display`; a sort field listed above for `sort_by`; `asc` or `desc` for `sort_order`; and a duration of at least `2s` for `refresh_interval`. `subscriptions` defaults to `false`; use `--subscriptions` to enable it for one run or `--subscriptions=false` to override an enabled config. If `sort_order` is omitted, the selected field's default direction is used. Command-line options override config values, which override built-in defaults. `refresh_interval` changes the live screen interval; it does not enable the live screen in a non-interactive terminal.

## Install

The CLI requires Linux, Windows, or macOS on amd64 or arm64, an internet connection, and a Twitch account. Approve `user:read:follows` during sign-in to list followed channels. Subscription details also require `user:read:subscriptions` and are only requested when enabled.

Download and extract a platform archive from GitHub Releases when a version is available, then put the executable on your `PATH`. The `--open` option uses `open` on macOS, `rundll32` on Windows, and `xdg-open` on Linux; Linux users may need to install `xdg-utils`.

Or install from a source checkout with Go:

```sh
go install .
```

## Sign-in and credentials

The CLI uses Twitch's Device Code Grant. It shows a verification link and one-time code, then tries to open the link in your browser. Sign in, enter the code, and approve the requested permissions.

Credentials are saved in your operating system's secure credential store: Keychain on macOS, Windows Credential Manager on Windows, or a Secret Service-compatible keyring on Linux. If the Linux keyring is unavailable, use `--no-store` for a session without saved credentials. Tokens are not written to a local file.

## Troubleshooting

- **The browser did not open:** copy the verification link into a browser and enter the displayed code.
- **The credential store is unavailable:** install and unlock the platform store, or use `--no-store`.
- **The follows permission is missing:** run `twitch-live-cli --logout`, then sign in again and approve it.
- **Subscription checks fail:** confirm the account can check subscriptions and approve `user:read:subscriptions`. Results are cached for the current process; restart the CLI to fetch them again.
- **Twitch reports a rate limit:** wait until the reset time shown in the error and retry.
- **Network or API error:** check your connection and try again.

## Development

Use Go 1.22 or newer. From the repository root, run:

```sh
go test ./...
go build -buildvcs=false ./...
```

GitHub Actions tests and builds on Linux, Windows, and macOS. Pushing a `v*` tag builds release archives for amd64 and arm64 on all three platforms.

To use your own Twitch application, set `TWITCH_CLIENT_ID` or pass `--client-id ID`. The app must support public-client Device Code Grant and request `user:read:follows`. Subscription support also requests `user:read:subscriptions`. Do not add a client secret to the CLI.
