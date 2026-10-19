# TwitchLiveCLI

See which of your followed Twitch channels are live from your terminal.

## Install

- **Platforms:** Linux, Windows, or macOS on amd64 or arm64.
- **Required:** internet access and a Twitch account.
- **Optional:** Streamlink with mpv or VLC for playback; Chatterino for chat. Keep these programs on your `PATH`.

Download and extract an archive from [GitHub Releases](https://github.com/thomasjdelaney/TwitchLiveCLI/releases), then put the executable on your `PATH`.

Or install from a source checkout with Go 1.26 or newer:

```sh
go install .
```

The browser viewer opens links with `open` on macOS, `rundll32` on Windows, and `xdg-open` on Linux. Linux may require `xdg-utils`.

## Get started

Run `twitch-live-cli`:

- In an interactive terminal, it opens the live screen and refreshes every minute.
- In a non-interactive terminal, it prints a snapshot by default.
- On first use, follow the sign-in link and enter the code shown in the terminal. Approve `user:read:follows` to list followed channels.

**Live screen controls**

- `↑`/`↓` or `j`/`k`: select a channel
- `Enter`: open it with your configured viewer (browser by default)
- `r`: refresh; `q`: quit
- `Home`/`End`: select the first/last channel; `PgUp`/`PgDn`: scroll

Channels are sorted by viewer count, highest first, by default.

## Commands

| Command | What it does |
| --- | --- |
| `twitch-live-cli --once` | Print one snapshot, even in an interactive terminal. |
| `twitch-live-cli --json` | Print live channels as JSON. |
| `twitch-live-cli --display card` | Use cards instead of the default table. |
| `twitch-live-cli --sort-by channel --sort-order asc` | Sort channels alphabetically. |
| `twitch-live-cli --subscriptions` | Include subscription status and tier. |
| `twitch-live-cli --subscriptions --sort-by sub_tier` | Sort by subscription tier. |
| `twitch-live-cli --viewer streamlink` | Use Streamlink with mpv to open selected streams. |
| `twitch-live-cli --viewer streamlink --player vlc` | Use VLC instead of mpv. |
| `twitch-live-cli --chat --open twitchdev` | Open Chatterino alongside the live channel. |
| `twitch-live-cli --viewer streamlink --quality 720p60` | Choose a Streamlink quality. |
| `twitch-live-cli --viewer streamlink --low-latency` | Enable Streamlink's Twitch low-latency mode. |
| `twitch-live-cli --refresh-interval 2m` | Refresh the live screen every two minutes (minimum `2s`). |
| `twitch-live-cli --config PATH` | Read preferences from a TOML file. |
| `twitch-live-cli --open twitchdev` | Open a followed channel if it is live. |
| `twitch-live-cli --logout` | Remove saved Twitch credentials. |
| `twitch-live-cli --no-store` | Sign in without saving credentials. |
| `twitch-live-cli --help` | Show all options. |

**Sorting**

- Fields: `viewers`, `channel`, `title`, `category`, `live_duration`, `subscribed`, `sub_tier`
- Default: viewers, descending
- `channel`, `title`, and `category` default to ascending; other fields default to descending.
- Sorting also applies to JSON output.
- `subscribed` and `sub_tier` require subscription details, enabled with `--subscriptions` or `subscriptions = true` in the config.
- `--json` cannot be combined with `--display`.
- `--open LOGIN` opens a channel only if it is live among your follows.

**Subscriptions**

- Off by default; enabling them requests Twitch's `user:read:subscriptions` permission in addition to `user:read:follows`.
- If saved authorization lacks this permission, the CLI asks you to authorize it.
- Subscription data is cached for the current process.

**Playback and chat**

- Browser is the default viewer.
- Streamlink playback requires Streamlink and mpv (default) or VLC on your `PATH`.
- Default quality is the lower middle video quality reported by Streamlink. Audio-only and symbolic qualities are skipped; if Streamlink reports qualities but none have a recognizable video resolution, `worst` is used.
- Set `--quality NAME` to skip quality discovery.
- `--chat` opens Chatterino separately alongside the channel. It works with either viewer and remains open independently of playback.
- Low-latency mode requires `viewer = "streamlink"`. It may buffer if the broadcaster has not enabled low-latency streaming; the player's cache may also add delay.
- In the live screen, the TUI stays open and refreshes while browser or Streamlink playback runs. You can open multiple Streamlink players, and they continue if you quit the TUI.
- With Streamlink, `--open LOGIN` runs playback in the current terminal and waits for it to exit.

## Configuration

Create `config.toml` to save preferences:

```toml
display = "card"
sort_by = "channel"
sort_order = "asc"
refresh_interval = "2m"
subscriptions = true
viewer = "streamlink"
player = "mpv"
chat = true
quality = "middle"
low_latency = false
```

Load a file from the current directory with:

```sh
twitch-live-cli --config ./config.toml
```

- Automatic path: `twitch-live-cli/config.toml` inside the operating system's user config directory.
- Precedence: command-line options override config values, which override built-in defaults.
- Use `--chat=false` to disable `chat = true` for one run.
- Valid `display`: `table` or `card`; `sort_order`: `asc` or `desc`; refresh interval: at least `2s`.
- `subscriptions` defaults to `false` and requires Twitch's `user:read:subscriptions` permission.
- `viewer` accepts `browser` or `streamlink`; `player` accepts `mpv` or `vlc`.
- Playback settings (`viewer`, `player`, `quality`, `chat`, `low_latency`) affect channel opening. Streamlink settings do not apply to the browser viewer.
- `refresh_interval` changes the live screen; it does not enable live mode in a non-interactive terminal.

## Sign-in and credentials

- Sign-in uses Twitch Device Code Grant: open the verification link, enter the displayed code, and approve the requested permissions.
- Credentials are stored in the platform's secure credential store: macOS Keychain, Windows Credential Manager, or a Secret Service-compatible keyring on Linux.
- Use `--no-store` for a run without saved credentials if the Linux keyring is unavailable.
- Tokens are not written to a local file.

## Troubleshooting

- **The browser did not open:** copy the verification link into a browser and enter the displayed code.
- **Credential store unavailable:** install and unlock the platform store, or use `--no-store`.
- **Follows permission missing:** run `twitch-live-cli --logout`, then sign in again and approve it.
- **Subscription checks fail:** confirm your account can check subscriptions and approve `user:read:subscriptions`; restart the CLI to refresh cached data.
- **Rate limit:** wait until the reset time shown in the error, then retry.
- **Network or API error:** check your connection and try again.

## Development

- Use Go 1.26 or newer.
- Run checks from the repository root:

  ```sh
  go test ./...
  go build -buildvcs=false ./...
  ```

- GitHub Actions tests and builds on Linux, Windows, and macOS.
- Pushing a `v*` tag builds release archives for amd64 and arm64 on all three platforms.
- See [CI investigation notes](docs/ci-investigation.md) for the Windows test startup investigation and diagnostic steps.
- To use your own Twitch application, set `TWITCH_CLIENT_ID` or pass `--client-id ID`. It must support public-client Device Code Grant and request `user:read:follows`; subscription support also requests `user:read:subscriptions`. Do not add a client secret to the CLI.
