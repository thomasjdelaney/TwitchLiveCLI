# TwitchLiveCLI

See which of your followed Twitch channels are live from your terminal. The command is `twitch-live-cli`.

## Install

The CLI requires Linux, Windows, or macOS on amd64 or arm64, an internet connection, and a Twitch account. Approve `user:read:follows` during sign-in to list followed channels. Subscription details also require `user:read:subscriptions` and are only requested when enabled. Streamlink playback additionally requires Streamlink and mpv or VLC; Chatterino is needed only when chat is enabled. These programs must be available on your `PATH`.

Download and extract a platform archive from [GitHub Releases](https://github.com/thomasjdelaney/TwitchLiveCLI/releases), then put the executable on your `PATH`. When using the browser viewer, the CLI opens links with `open` on macOS, `rundll32` on Windows, and `xdg-open` on Linux; Linux users may need to install `xdg-utils`. With the Streamlink viewer, `--open LOGIN` runs Streamlink in the current terminal and waits for playback to exit.

Or install from a source checkout with Go 1.26 or newer:

```sh
go install .
```

## Get started

Run `twitch-live-cli` in an interactive terminal to open the live screen. It refreshes every minute by default. Use the arrow keys or `j`/`k` to select a channel, press Enter to open it with your configured viewer (the browser by default), `r` to refresh, and `q` to quit. Home/End select the first or last channel; PgUp/PgDn scroll. Without an interactive terminal, it prints one snapshot by default.

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
| `twitch-live-cli --viewer streamlink` | Open selected streams in Streamlink with mpv. |
| `twitch-live-cli --viewer streamlink --player vlc` | Use VLC instead of mpv for Streamlink playback. |
| `twitch-live-cli --chat --open twitchdev` | Open Chatterino alongside the live channel; in the live screen, Chatterino opens when you open a selected channel. |
| `twitch-live-cli --viewer streamlink --quality 720p60` | Use a specific Streamlink quality. |
| `twitch-live-cli --viewer streamlink --low-latency` | Enable Streamlink's Twitch low-latency mode. |
| `twitch-live-cli --refresh-interval 2m` | Refresh the live screen every two minutes. Minimum: `2s`. |
| `twitch-live-cli --config PATH` | Read preferences from a TOML file. |
| `twitch-live-cli --open twitchdev` | Open a followed channel if it is live. |
| `twitch-live-cli --logout` | Remove saved Twitch credentials. |
| `twitch-live-cli --no-store` | Sign in without saving credentials. |
| `twitch-live-cli --help` | Show all options. |

Sort fields are `viewers`, `channel`, `title`, `category`, `live_duration`, `subscribed`, and `sub_tier`. The default is viewers descending; channel, title, and category default to ascending, and the other fields to descending. Sorting also applies to JSON output. `subscribed` and `sub_tier` require subscription details to be enabled. `--json` cannot be combined with `--display`. `--open` takes a channel login, such as `twitchdev`, and opens it only if it is currently live among your followed channels.

Subscription details are off by default. Enabling them requests Twitch's `user:read:subscriptions` permission in addition to `user:read:follows`. If your saved authorization lacks the subscription permission, the CLI asks you to authorize it.

The browser is the default viewer. Streamlink playback requires Streamlink and the selected player (`mpv` by default, or `vlc`) on your `PATH`. By default, the CLI chooses the lower middle video resolution among the qualities Streamlink reports for that channel; audio-only and symbolic names are ignored. If Streamlink reports qualities but none have recognizable video resolutions, it uses `worst`. Set `--quality NAME` to skip discovery and choose a specific Streamlink quality. `--chat` opens Chatterino separately and leaves it running after playback ends; it works with either viewer.

Low-latency mode is available with `viewer = "streamlink"`. It can cause buffering when the broadcaster has not enabled Twitch low-latency streaming, and the player's own cache may still add delay. In the live screen, browser and Streamlink playback open separately while the TUI remains usable and continues refreshing; you can open more than one Streamlink player. Playback continues if you quit the TUI. With the Streamlink viewer, `--open LOGIN` runs Streamlink in the current terminal and waits for it to exit.

## Advanced usage

You can save preferences in a TOML file instead of passing the same options every time. Create a file named `config.toml` with the settings you want:

```toml
# Display channels as cards, sorted alphabetically.
display = "card"
sort_by = "channel"
sort_order = "asc"

# Refresh the live screen every two minutes.
refresh_interval = "2m"

# Optional Twitch subscription information and playback settings.
subscriptions = true
viewer = "streamlink"
player = "mpv"
chat = true
quality = "middle"
low_latency = false
```

To use this file from the current directory, pass its path:

```sh
twitch-live-cli --config ./config.toml
```

You can also place it at `twitch-live-cli/config.toml` inside your operating system's user config directory. The CLI reads that file automatically when `--config` is omitted. Command-line options override values in the config, and config values override built-in defaults. For example, `--chat=false` disables `chat = true` for one run.

Use `table` or `card` for `display`; a sort field listed above for `sort_by`; `asc` or `desc` for `sort_order`; and a duration of at least `2s` for `refresh_interval`. If `sort_order` is omitted, the selected field's default direction is used. `subscriptions` defaults to `false` and requires Twitch's `user:read:subscriptions` permission when enabled. `viewer` accepts `browser` or `streamlink`; `player` accepts `mpv` or `vlc`. `viewer`, `player`, `quality`, `chat`, and `low_latency` affect channel opening; Streamlink settings have no effect with the browser viewer. `refresh_interval` changes the live screen interval; it does not enable the live screen in a non-interactive terminal.

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

Use Go 1.26 or newer. From the repository root, run:

```sh
go test ./...
go build -buildvcs=false ./...
```

GitHub Actions tests and builds on Linux, Windows, and macOS. Pushing a `v*` tag builds release archives for amd64 and arm64 on all three platforms.

See [CI investigation notes](docs/ci-investigation.md) for the findings and diagnostic steps from the Windows test startup issue.

To use your own Twitch application, set `TWITCH_CLIENT_ID` or pass `--client-id ID`. The app must support public-client Device Code Grant and request `user:read:follows`. Subscription support also requests `user:read:subscriptions`. Do not add a client secret to the CLI.
