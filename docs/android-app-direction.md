# Android app direction

## What has been tried

The Linux build runs well in Termux on Android. It can also launch a live stream in the Twitch app, which is a useful viewing flow.

Termux is a poor installation path for a general audience, though: installing it required disabling two Google Play security features. That friction is likely to put off many users.

## Direction

If the goal is an Android version that most people can install comfortably, pursue a native Android app rather than asking users to install Termux. Preserve the ability to hand off stream viewing to the Twitch app; an in-app video player is not necessary for the initial concept.

The native app would need a mobile interface for followed channels and live status, plus Android implementations for sign-in and secure credential storage, opening streams, notifications, and settings. The existing Go code may provide reusable Twitch API and channel-monitoring logic, while the terminal UI and Streamlink integration are not a direct fit for the mobile experience.

This is an exploratory direction, not a committed implementation plan. The scope and choice of Android technology still need to be worked out before development.
