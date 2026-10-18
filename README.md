# live-followed-cli

`live-followed-cli` checks which of your followed Twitch channels are live. It uses Twitch Helix and displays channel, title, category, viewers, and stream duration in a compact terminal view.

## Install

When a version tag is pushed, GitHub Actions builds release archives for Linux, Windows, and macOS (amd64 and arm64) and attaches them to the repository's GitHub release. Download the archive for your operating system and architecture, extract it, and place the executable somewhere on your `PATH`.

With Go installed, you can also install from a source checkout on Linux, Windows, or macOS:

```sh
go install .
```

To build a standalone executable, use the command for your platform (the explicit flag also works in source exports without Git metadata):

```text
Linux:   go build -buildvcs=false -o live-followed-cli .
macOS:   go build -buildvcs=false -o live-followed-cli .
Windows: go build -buildvcs=false -o live-followed-cli.exe .
```

Copy the resulting executable to a directory on your `PATH`. The same output names work in Bash, PowerShell, and Command Prompt.

## First run and authentication

Run:

```sh
live-followed-cli
```

On first use, the CLI prints a Twitch verification URL and one-time code, then opens the URL in your browser when possible. Sign in to Twitch, enter the code, and authorize the `user:read:follows` permission. No client secret or callback server is used. The registered redirect URL is not used by Device Code Grant.

The CLI uses Twitch's public-client Device Code Grant and does not send or require a client secret. If Twitch rejects device authorization for your app, check the app's OAuth client configuration in the Developer Console.

The default application ID is the one distributed with this project. To use another Twitch application, pass `--client-id ID` or set `TWITCH_CLIENT_ID`. Only application client IDs are public; never put a client secret in the CLI or share OAuth tokens.

## Credential storage

Access and refresh tokens are saved in the operating system credential manager:

- **macOS:** Keychain. The first save or access may require you to approve Keychain access.
- **Windows:** Windows Credential Manager, using the current Windows user account.
- **Linux:** a Secret Service-compatible keyring such as GNOME Keyring or KDE Wallet, with a running DBus session. Install and unlock the keyring using your desktop environment. On headless Linux, start a Secret Service or use `--no-store`.

If the credential store is unavailable, the CLI explains the issue and exits without writing tokens to a file. `--no-store` opts into session-only credentials; you will need to sign in again on the next run. Use `--logout` to delete stored credentials.

Public-client refresh tokens expire after 30 days. If Twitch rejects an expired or revoked authorization, run the command again to authorize.

## Usage

```text
live-followed-cli [flags]

  --json                 write live channels as JSON
  --watch DURATION       refresh periodically (for example, 2m)
  --open LOGIN           open a followed live channel in your browser
  --client-id ID         use a different Twitch application ID
  --no-store             do not persist tokens in the OS credential store
  --logout               remove saved Twitch credentials
```

Examples:

```sh
live-followed-cli
live-followed-cli --watch 90s
live-followed-cli --json
live-followed-cli --open twitchdev
live-followed-cli --logout
```

Channels are sorted by current viewer count. When no followed channel is live, the CLI shows an empty state; JSON mode returns an empty array. `--open` requires an exact channel login among the live results. Browser opening uses `open` on macOS, `rundll32` on Windows, and `xdg-open` on Linux; install `xdg-utils` on Linux if needed.

## Troubleshooting

- **Credential store unavailable:** install and unlock the platform store described above, or use `--no-store` for one session.
- **Authorization missing the follows permission:** use `--logout`, then run the CLI and approve the requested permission again.
- **Rate limit reached:** wait until the reset time printed by the CLI and retry. The CLI batches stream lookups and does not send one request per channel.
- **Browser did not open:** copy the printed URL into a browser and enter the displayed code.
- **Network or Twitch error:** check connectivity and retry. Twitch response errors are shown without printing OAuth credentials.

## Development

```sh
gofmt -w *.go
go test ./...
```

CI runs tests on Linux, Windows, and macOS and builds release archives for Linux, Windows, and macOS on amd64 and arm64 when a version tag is pushed. Release downloads are attached to the GitHub release for this repository.
