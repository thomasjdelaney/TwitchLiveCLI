# TwitchLiveCLI

Check which of your followed Twitch channels are live from your terminal. The command is `twitch-live-cli`.

## Use the CLI

Run `twitch-live-cli` in an interactive terminal to open the live screen. It refreshes every minute. Use the arrow keys or `j`/`k` to select a channel, then press Enter to open it in your browser. Home/End select the first/last channel; PgUp/PgDn scroll the list. Press `r` to refresh or `q` to quit. Without an interactive terminal, it prints one snapshot.

On first run, follow the sign-in link and enter the code shown in the terminal. Live followed channels appear in a table sorted by viewer count. If none are live, the CLI says so.

| Command | What it does |
| --- | --- |
| `twitch-live-cli --once` | Print one snapshot. |
| `twitch-live-cli --json` | Print live channels as JSON. |
| `twitch-live-cli --display card` | Show channels as cards. The default is a table. |
| `twitch-live-cli --sort-by channel --sort-order asc` | Choose a sort field and direction. |
| `twitch-live-cli --refresh-interval 2m` | Refresh the live screen every two minutes. |
| `twitch-live-cli --config PATH` | Read preferences from a TOML file. |
| `twitch-live-cli --open twitchdev` | Open that followed channel if it is live. |
| `twitch-live-cli --logout` | Remove saved Twitch credentials. |
| `twitch-live-cli --no-store` | Sign in without saving credentials. |
| `twitch-live-cli --help` | Show all options. |

Sort fields are `viewers`, `channel`, `title`, `category`, and `live_duration`. The default is viewers descending; channel, title, and category default to ascending, and live duration to descending. Sorting also applies to JSON. Do not combine `--json` with `--display`. `--open` takes a channel login, such as `twitchdev`.

## Preferences

Preferences are read from `twitch-live-cli/config.toml` in your operating system's user config directory. The file is optional. Use `--config PATH` to choose another file. For example:

```toml
display = "card"
sort_by = "channel"
sort_order = "asc"
refresh_interval = "2m"
```

Use `table` or `card` for `display`, one of the sort fields above for `sort_by`, `asc` or `desc` for `sort_order`, and a duration of at least `2s` for `refresh_interval`. If omitted, sort direction follows the selected field's default. Command-line options override config values, which override built-in defaults. `refresh_interval` changes the live screen interval; it does not turn on the live screen in a non-interactive terminal. The same 2-second minimum applies to `--refresh-interval`.

## Requirements and installation

- Linux, Windows, or macOS on amd64 or arm64.
- An internet connection and a Twitch account. To list followed channels, approve the `user:read:follows` permission during sign-in.
- An interactive terminal for the live screen. Without one, the command prints a snapshot; use `--json` for machine-readable output.

Download a platform archive from this repository's GitHub Releases page when a version is published, extract it, and put the executable on your `PATH`. Or install from a source checkout with Go:

```sh
go install .
```

To build a standalone executable, run the command for your platform:

```text
Linux:   go build -buildvcs=false -o twitch-live-cli .
macOS:   go build -buildvcs=false -o twitch-live-cli .
Windows: go build -buildvcs=false -o twitch-live-cli.exe .
```

The `--open` option uses `open` on macOS, `rundll32` on Windows, and `xdg-open` on Linux; install `xdg-utils` on Linux if needed.

## Sign-in and credentials

The CLI uses Twitch's Device Code Grant. It prints a verification link and one-time code, then tries to open the link in your browser. Sign in, enter the code, and approve the `user:read:follows` permission.

Credentials are saved in your operating system's secure credential store: Keychain on macOS, Windows Credential Manager on Windows, or a Secret Service-compatible keyring on Linux. If the Linux keyring is unavailable, use `--no-store` for a session without saved credentials. Tokens are not written to a local file.

## Troubleshooting

- **The browser did not open:** copy the verification link from the terminal into a browser and enter the displayed code.
- **The credential store is unavailable:** install and unlock the platform store, or run with `--no-store`.
- **The follows permission is missing:** run `twitch-live-cli --logout`, then sign in again and approve it.
- **Twitch reports a rate limit:** wait until the reset time shown in the error and retry.
- **Network or API error:** check your connection and try again.

## Development

Use Go 1.22 or newer. From the repository root, run:

```sh
go test ./...
go build -buildvcs=false ./...
```

The GitHub Actions workflow tests and builds on Linux, Windows, and macOS. Pushing a `v*` tag builds release archives for amd64 and arm64 on all three platforms.

To use your own Twitch application during development, set `TWITCH_CLIENT_ID` or pass `--client-id ID`. The application must support public-client Device Code Grant and request the `user:read:follows` scope. Do not add a client secret to the CLI.
