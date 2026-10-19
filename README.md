# TwitchLiveCLI

See which of your followed Twitch channels are live from your terminal.

## Contents

- [Install](#install)
- [Sign in and run](#sign-in-and-run)
- [Commands](#commands)
- [Configuration](#configuration)
- [Playback and chat](#playback-and-chat)
- [Credentials](#credentials)
- [Troubleshooting](#troubleshooting)
- [Development](#development)

## Install

Requirements: Linux, macOS, or Windows on amd64 or arm64; internet access; and a Twitch account.

### Linux and macOS

Install the latest release with `curl`:

```sh
curl -fsSL https://raw.githubusercontent.com/thomasjdelaney/TwitchLiveCLI/master/install.sh | sh
```

Or use `wget`:

```sh
wget -qO- https://raw.githubusercontent.com/thomasjdelaney/TwitchLiveCLI/master/install.sh | sh
```

The installer puts the binary in `~/.local/bin`. Set `INSTALL_DIR` to choose another location:

```sh
curl -fsSL https://raw.githubusercontent.com/thomasjdelaney/TwitchLiveCLI/master/install.sh | INSTALL_DIR="$HOME/bin" sh
```

### Windows

Run this in PowerShell to install the latest release:

```powershell
$installer = Join-Path ([IO.Path]::GetTempPath()) "twitch-live-cli-install-$([guid]::NewGuid()).ps1"
try {
    Invoke-WebRequest https://raw.githubusercontent.com/thomasjdelaney/TwitchLiveCLI/master/install.ps1 -OutFile $installer -ErrorAction Stop
    & $installer
} finally {
    Remove-Item -LiteralPath $installer -Force -ErrorAction SilentlyContinue
}
```

It installs to `%LOCALAPPDATA%\Programs\twitch-live-cli` and adds that directory to your user `PATH`. Open a new terminal after installation. To choose another directory, run the downloaded script with `-InstallDir PATH`. The installer requires `tar.exe`.

### From source

With Go 1.26 or newer:

```sh
go install .
```

## Sign in and run

```sh
twitch-live-cli
```

- In an interactive terminal, the live screen refreshes every minute.
- Otherwise, the command prints one snapshot.
- On first run, open the verification link, enter the displayed code, and approve access to followed channels.

The live screen sorts by viewer count, highest first. Use `↑`/`↓` or `j`/`k` to select a channel, `Enter` to open it, `r` to refresh, and `q` to quit. `Home`/`End` and `PgUp`/`PgDn` navigate longer lists.

## Commands

| Example | Purpose |
| --- | --- |
| `twitch-live-cli --once` | Print one snapshot in any terminal. |
| `twitch-live-cli --json` | Print live channels as JSON. |
| `twitch-live-cli --display card` | Show cards instead of the default table. |
| `twitch-live-cli --sort-by channel --sort-order asc` | Sort alphabetically. |
| `twitch-live-cli --subscriptions` | Include subscription status and tier. |
| `twitch-live-cli --refresh-interval 2m` | Set the live screen refresh interval (minimum `2s`). |
| `twitch-live-cli --open twitchdev` | Open a followed channel if it is live. |
| `twitch-live-cli --logout` | Remove saved Twitch credentials. |
| `twitch-live-cli --version` | Show the installed version. |
| `twitch-live-cli --update` | Update to the latest release. The executable's directory must be writable; on Windows, the update completes after exit. |
| `twitch-live-cli --no-store` | Keep credentials only for the current run. |
| `twitch-live-cli --help` | List all options. |

Sort fields: `viewers`, `channel`, `title`, `category`, `live_duration`, `subscribed`, and `sub_tier`. Text fields default to ascending order; other fields default to descending. Subscription sorting requires `--subscriptions`. JSON output cannot be combined with `--display`.

## Configuration

Save preferences in `config.toml`:

```toml
display = "card"
sort_by = "channel"
sort_order = "asc"
refresh_interval = "2m"
subscriptions = false
viewer = "browser"
```

Load a specific file with `twitch-live-cli --config ./config.toml`. By default, preferences are read from `twitch-live-cli/config.toml` in your operating system's user config directory. Command-line options override config values, which override defaults.

Supported settings include `display` (`table` or `card`), `sort_by`, `sort_order` (`asc` or `desc`), `refresh_interval` (at least `2s`), `subscriptions`, `viewer` (`browser` or `streamlink`), `player` (`mpv` or `vlc`), `quality`, `chat`, and `low_latency`. Use `--chat=false` to override `chat = true`. Refresh settings affect the interactive screen only.

## Playback and chat

- The default viewer opens streams in your browser.
- Streamlink playback requires [Streamlink](https://streamlink.github.io/) and mpv or VLC on your `PATH`.
- Choose a player or quality with `--player vlc` and `--quality 720p60`.
- `--low-latency` enables Streamlink's Twitch low-latency mode; set `--viewer streamlink` to use it.
- `--chat` opens [Chatterino](https://chatterino.com/) alongside the stream. Both tools must be on your `PATH`.
- Opening links uses `open` (macOS), `rundll32` (Windows), or `xdg-open` (Linux). Linux may require `xdg-utils`.

## Credentials

The CLI uses Twitch Device Code Grant. It stores credentials in the system credential store: Keychain on macOS, Credential Manager on Windows, or a Secret Service keyring on Linux. Use `--no-store` to keep credentials only for the current process, or `--logout` to remove saved credentials.

Subscription details are off by default. Enabling `--subscriptions` requests Twitch's `user:read:subscriptions` permission in addition to `user:read:follows`.

## Troubleshooting

- **Sign-in page did not open:** open the verification link yourself and enter the displayed code.
- **Credential store unavailable:** unlock or install your system keyring, or run with `--no-store`.
- **Follows permission missing:** run `twitch-live-cli --logout`, then sign in again.
- **Subscription lookup fails:** confirm your account can check subscriptions and approve the requested permission.
- **Rate limit:** wait until the reset time in the error message, then retry.

## Development

- Requires Go 1.26 or newer.
- Run checks from the repository root: `go test ./...` and `go build -buildvcs=false ./...`.
- GitHub Actions tests on Linux, Windows, and macOS. Version tags (`v*`) build release archives for amd64 and arm64 on all three platforms.
- See [CI investigation notes](docs/ci-investigation.md) for the Windows test startup investigation.
- To use your own Twitch application, set `TWITCH_CLIENT_ID` or pass `--client-id ID`. It must support public-client Device Code Grant. Never add a client secret to the CLI.
