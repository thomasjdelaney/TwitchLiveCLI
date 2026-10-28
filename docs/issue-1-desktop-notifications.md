# Desktop notifications

## Implemented

A simplified version of the original notification proposal is available in the interactive live screen. Users can enable notifications for followed channels individually, either in the notification manager or by editing the TOML config. Open the manager with `n` from the live screen or run `twitch-live-cli notifications` in an interactive terminal. Channel selections are saved to the active config file.

Notifications are disabled for all channels by default and are delivered only while the regular live screen is running. By default, a notification is sent when an enabled channel goes live. The optional startup alert also notifies for enabled channels already live when the live screen starts. Notifications include the channel name, stream title, category, and live duration when available.

## Possible future enhancements

The following ideas from the original proposal are not implemented. They may be considered for future development; this list is not a commitment:

- One-time or recurring weekly watch schedules per channel.
- User-configurable notification display duration.
- Further schedule controls, such as defining behavior across midnight and daylight-saving changes.

Monitoring remains limited to the interactive live screen. There is no background service after the CLI exits.
