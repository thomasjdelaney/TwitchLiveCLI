# CI investigation notes

## Windows test startup hang

### Finding

Windows CI's test step stalled before reporting individual tests. A temporary marker in `TestMain` did not appear, which showed that the test runner had not entered `m.Run()`. This pointed to work happening before test execution, such as package or dependency initialization, rather than a slow or failing test assertion.

The investigation narrowed the change to the LipGloss compatibility path. Windows runs passed after the compatibility import was removed and LipGloss was pinned to `charm.land/lipgloss/v2 v2.0.5`. The evidence supports that change as the practical fix, but the exact initializer was not isolated with a minimal reproducer, so treat the low-level cause as unconfirmed.

### Current safeguards

- Keep LipGloss pinned at 2.0.5 unless a deliberate dependency update is tested on Windows.
- Do not reintroduce the LipGloss `compat` import without verifying Windows CI. The current renderers use LipGloss v2 styles directly.
- The Windows CI step uses `go test -json -count=1 -timeout=4m ./...`. The JSON output retains test-level progress and the timeout prevents an indefinite run.
- The temporary `TestMain` marker and `GODEBUG=inittrace=1` setting were investigation tools; they are not part of the normal workflow.

### If the hang returns

1. Inspect the Windows Actions log and determine whether any individual test starts.
2. If no test starts, check whether a temporary marker at the start of `TestMain` is reached. If it is not, investigate test binary startup and package initialization before editing test cases.
3. Separate compilation from execution with `go test -c -x -o twitch-live-cli.test.exe .`, then run the binary with `-test.v` and a bounded `-test.timeout` to see whether the stall occurs before or during tests.
4. Temporarily use `GODEBUG=inittrace=1` on the Windows run to inspect package initialization progress. Remove the setting and any marker after the diagnosis; init tracing is noisy and not needed on every CI run.
5. Compare the dependency graph and imports with the last passing run, especially changes to terminal styling and compatibility packages. Confirm the proposed fix with a fresh Windows run before recording it as resolved.

## Terminal color behavior

The interactive screen asks the terminal for its background color and selects the matching palette. If the terminal cannot report a background, the interface starts with neutral styling. One-shot output also stays neutral and does not paint row backgrounds, so it remains readable in both light and dark terminal profiles. Keep theme colors in an explicit palette passed to renderers rather than changing package-global styles.
