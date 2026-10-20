# Twitch public client migration

The built-in Twitch application is now registered as a **Public** client. The CLI uses Device Code Grant and refreshes tokens without a client secret, which is appropriate for an application installed on users' computers. Never add a client secret to the CLI.

## Existing installations

Credentials saved by an earlier version are tied to the previous client ID. After updating, each existing user needs to remove those credentials and sign in again once:

```sh
twitch-live-cli --logout
twitch-live-cli
```

New installations already use the public app and can sign in normally. Users who set `TWITCH_CLIENT_ID` or pass `--client-id` should make sure their chosen app supports public-client Device Code Grant. If they change client IDs, they need to log out and sign in again because saved tokens are tied to the client ID that issued them.

For details, see Twitch's [Device Code Grant documentation](https://dev.twitch.tv/docs/authentication/getting-tokens-oauth) and [refresh token documentation](https://dev.twitch.tv/docs/authentication/refresh-tokens/).
