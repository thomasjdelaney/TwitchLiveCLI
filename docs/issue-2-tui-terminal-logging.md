# Issue #2: Live TUI display corruption

## Diagnosis

The report describes stale or misplaced rows that disappear after a terminal resize, along with `saved access token is valid` printed over the live screen. The reporter also found that redirecting stderr stopped the corruption. This supports terminal output from outside Bubble Tea's renderer as the likely cause, though the long-running failure has not been independently reproduced.

OAuth validation runs after the saved token has gone an hour without validation, which matches the delayed log message. Authentication and Twitch API 401 diagnostics previously wrote directly to stderr, which can share the terminal with the alternate-screen TUI. The resize-triggered repaint is consistent with the renderer recovering from out-of-band terminal output.

The report also shows duplicate stream rows. `LiveFollowedSorted` appends results from multiple Twitch API batches without deduplicating them, but there is no evidence that the API returned duplicates. The logging fix does not alter stream data or row handling; inspect API results separately if duplicate rows continue.

## Fix

- OAuth diagnostics now use an optional writer, defaulting to stderr for commands that do not select another destination.
- The live TUI suppresses auth diagnostics unless `--auth-log PATH` is set.
- Snapshot output, including `--once`, `--json`, and automatic snapshots in noninteractive terminals, also suppresses auth diagnostics unless `--auth-log PATH` is set.
- `--auth-log PATH` appends diagnostics to the specified file. It does not redirect unrelated stderr output.
- Refresh scheduling, Twitch requests, and model updates are unchanged.

## Verification and remaining check

Tests cover configured logging, the `saved access token is valid` validation path, default stderr behavior, and suppressed snapshot output. The Go test suite and build pass. A manual long-running TUI session through token validation has not been performed; if the display corruption or duplicates persist, capture the API data and investigate those separately.
