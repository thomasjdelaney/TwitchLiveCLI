# Issue #1: Scheduled desktop notifications for followed channels

## Context

Issue #1 asks for desktop notifications so TwitchLiveCLI can replace Chatterino for a user whose main remaining reason to use Chatterino is its notifications. The user especially values that TwitchLiveCLI reads their Twitch follow list directly, rather than requiring them to reproduce it as tabs.

The feature discussed here is for the interactive live mode. TwitchLiveCLI already fetches followed channels and refreshes the live-channel list. The live screen is a Bubble Tea application built with Charm libraries, and preferences are stored in TOML. This document describes the intended feature; it is a design handoff, not a report that the feature has been implemented.

## User-facing behavior

- Show a cross-platform desktop notification when a configured channel is live during one of its watch windows. Include the channel name and current stream title.
- Let users configure notifications inside live mode. They can search/filter the followed-channel list, select channels, and manage schedules for individual channels. The selector must remain usable with a large follow list.
- Support multiple schedules per channel:
  - A one-time watch window that begins when it is created and lasts for a user-chosen duration (for example, “notify me if channel X goes live in the next two hours”).
  - A recurring weekly window with selected weekdays and local start/end times (for example, “notify me if channel X goes live or is live from 1pm to 3pm on Monday, Wednesday, and Thursday”).
- Allow users to add, edit, and remove schedules. Persist changes automatically so they survive restarts, using the same config file live mode loaded, including a file passed with `--config`.
- Let users configure how long desktop notifications should remain visible. The user can also dismiss them through the operating system’s normal notification UI.
- Start with notifications disabled for every channel; the user explicitly selects channels to monitor.

## Schedule and monitoring semantics

- Weekly schedules use the computer’s local time zone and follow its daylight-saving changes.
- The existing live-mode refresh interval controls how often Twitch is checked; do not add a separate notification polling interval.
- Monitoring happens only while live mode is running. Persist schedules, but do not run a background service when TwitchLiveCLI is closed.
- Notify once when a watch window begins if the channel is already live. Also notify once when live mode starts inside an active window if the channel is already live.
- During an active window, notify when a channel changes from offline to live. Do not repeat a notification on every refresh while it remains live. A later offline-to-live transition in the same active window may notify again.
- Treat a one-time window as starting immediately upon creation. Once it expires, it no longer triggers notifications.
- Weekly windows are evaluated using local weekday and clock time. Define and test behavior for windows that span midnight and daylight-saving transitions as part of implementation; user-facing schedule editing should make the resulting window clear.

## Implementation outline

### Preferences and config

Extend the existing preference/config types with notification display timeout and per-channel schedules. Preserve the current TOML loading, validation, and `--config` behavior. Save interactive changes automatically and report persistence errors in the live UI without silently losing the user’s edits. Keep schedule data associated with stable channel identity (prefer Twitch broadcaster ID, with login/name used for display) so a renamed channel does not lose its settings.

### Live-mode interface

Add a notification settings view to the existing Bubble Tea model. Provide a filterable followed-channel list, clear selection state, and per-channel schedule management for one-time and recurring weekly windows. Include keyboard help and usable empty/loading/error states. Keep the existing live-channel navigation and refresh behavior intact while users edit settings.

### Detection and notification delivery

Compare successful live-channel fetches with prior state and evaluate configured schedules at each refresh. Track enough state to avoid duplicate notices across refreshes and to implement the entry behavior above. Handle the initial fetch, manual refresh, refresh failures, schedule edits, and live-mode startup consistently; a failed Twitch refresh must not be mistaken for channels going offline.

Put desktop delivery behind a small notifier interface so detection and scheduling can be tested without displaying real desktop notifications. Select a maintained Go approach that supports Windows, macOS, and Linux. The configured timeout is best effort: operating systems and desktop environments can override display duration, and some provide only coarse timeout choices. Manual dismissal is provided by the native notification UI. Surface delivery failures in the live-mode status rather than crashing the TUI.

### Documentation

Update the README’s live-mode and configuration guidance with the settings workflow, schedule examples, local-time behavior, requirement that live mode stay running, and timeout limitations. Keep this design handoff as the detailed implementation reference.

## Acceptance and verification

- Config tests cover defaults, validation, serialization/round-trip of one-time and weekly schedules, and auto-save to both the default config path and an explicit `--config` path.
- Schedule tests cover active-window boundaries, selected weekdays, multiple schedules, expiry of one-time windows, midnight-crossing windows, and daylight-saving transitions.
- Detection tests use a fake clock, fake Twitch results, and a fake notifier to verify live transitions, window-entry notification for an already-live channel, startup inside a window, duplicate suppression, a later new live session, manual refresh, and recovery after failed refreshes.
- UI-model tests cover filtering a large channel list, selecting channels, adding/editing/removing schedules, persistence errors, and saving changes.
- Build for supported Windows, macOS, and Linux targets. Smoke-test native notifications and manual dismissal where those platforms are available; state clearly which targets could not be exercised interactively.
- Do not claim that notification timeout is exact across operating systems; verify and document the behavior provided by each chosen backend.

## Decisions and constraints

- Scope is desktop notifications in interactive live mode; noninteractive snapshots and JSON output do not monitor schedules.
- No notification channels are enabled by default.
- Watch windows are per channel and can be one-time or weekly; multiple schedules per channel are allowed.
- Weekly times use system local time. Existing refresh interval is the notification check cadence.
- Config changes made in the UI save automatically.
- Notification content is channel name and stream title. Click-to-open behavior, repeat reminders while continuously live, and background monitoring after the CLI exits are not part of this feature.
- Do not embed or introduce Twitch client secrets. Notifications use the existing followed-channel/live-stream API access.
