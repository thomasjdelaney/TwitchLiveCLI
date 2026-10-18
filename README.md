# TwitchLiveCLI

Check which of your followed Twitch channels are live from your terminal. The command is `twitch-live-cli`; it shows each live channel's title, category, viewer count, and time live.

## Usage

Run a quick check:

```sh
twitch-live-cli
```

On first run, sign in to Twitch using the code and link shown in the terminal. After authorization, the CLI shows live channels sorted by viewer count. If nobody you follow is live, it displays an empty-state message.

Useful options:

| Command | What it does |
| --- | --- |
| `twitch-live-cli --help` | Show all available options. |
| `twitch-live-cli --json` | Print the live channels as JSON. |
| `twitch-live-cli --table` | Show channels in a compact table. Works with `--live-mode` too. |
| `twitch-live-cli --live-mode` | Keep a live terminal screen updated every minute. Press `r` to refresh now, `q` to quit, and use the arrow keys or `j`/`k` to scroll. |
| `twitch-live-cli --open twitchdev` | Open that followed channel if it is live. |
| `twitch-live-cli --logout` | Remove saved Twitch credentials. |
| `twitch-live-cli --no-store` | Sign in without saving credentials; sign in again next time. |

Pass a duration to change the live mode interval, for example `twitch-live-cli --live-mode 2m`. In live mode, results update in place; if `--json` and `--live-mode` are combined, the CLI prints one JSON array per refresh. `--json` and `--table` cannot be combined. Long table cells are shortened to fit the terminal. `--open` expects a channel login, such as `twitchdev`.

## Requirements

- Linux, Windows, or macOS on amd64 or arm64.
- An internet connection and a Twitch account. To list channels, the account must follow channels and approve the `user:read:follows` permission during sign-in.
- A terminal that supports interactive input to use `--live-mode`. Other commands work without the interactive screen.

## Install

Download a platform archive from this repository's GitHub Releases page when a version is published, extract it, and put the executable in a directory on your `PATH`. Release builds target Linux, Windows, and macOS on amd64 and arm64.

Or install from a source checkout with Go:

```sh
go install .
```

To build a standalone executable, run the command for your platform:

```text
Linux:   go build -buildvcs=false -o twitch-live-cli .
macOS:   go build -buildvcs=false -o twitch-live-cli .
Windows: go build -buildvcs=false -o twitch-live-cli.exe .
```

Copy the executable to a directory on your `PATH`. The browser-open option uses `open` on macOS, `rundll32` on Windows, and `xdg-open` on Linux; install `xdg-utils` on Linux if needed.

## First-time sign-in and credential storage

The CLI uses Twitch's Device Code Grant. It prints a verification link and one-time code, then tries to open the link in your browser. Sign in, enter the code, and approve the `user:read:follows` permission.

The Twitch application ID is built in, so no app setup is needed for normal use. Developers who want to use their own Twitch application can find the configuration instructions in the Development section.

Credentials are saved in your operating system's secure credential store:

- **macOS:** Keychain. macOS may ask you to approve access.
- **Windows:** Windows Credential Manager, for your current Windows user.
- **Linux:** a Secret Service-compatible keyring such as GNOME Keyring or KDE Wallet. It must be installed, unlocked, and available over DBus. If it is unavailable, use `--no-store` for a session without saved credentials.

Tokens are not written to a local file. Twitch may require you to sign in again if your authorization is revoked or its refresh token expires.

## Troubleshooting

- **The browser did not open:** copy the verification link from the terminal into a browser and enter the displayed code.
- **The credential store is unavailable:** install and unlock the platform store above, or run with `--no-store`.
- **The follows permission is missing:** run `twitch-live-cli --logout`, then start the CLI again and approve the permission.
- **Twitch reports a rate limit:** wait until the reset time shown in the error and retry.
- **Network or API error:** check your connection and try again.

## Development

Build and test from the repository root with Go 1.22 or newer:

```sh
go test ./...
go build -buildvcs=false ./...
```

The GitHub Actions workflow runs tests and builds on Linux, Windows, and macOS. Pushing a `v*` tag builds release archives for amd64 and arm64 on all three platforms.

To develop with your own Twitch application, set `TWITCH_CLIENT_ID` or pass `--client-id ID`. The application must support public-client Device Code Grant and request the `user:read:follows` scope. Do not add a client secret to the CLI.
