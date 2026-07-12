---
title: First login and roles
description: Log in as admin the first time, enable the optional read-only viewer role, and understand what each role can see and do.
sidebar:
  order: 3
---

This page covers logging in for the first time, the difference between the admin and viewer roles, and how to enable the read-only viewer login.

## Log in as admin

Open the panel at the private address you published, for example `http://127.0.0.1:8080`. You will see a login screen.

Log in with the password you set in `PALHELM_ADMIN_PASSWORD`. There is no username. The admin login is the account you set that env var for.

On a successful login, Palhelm sets an HttpOnly session cookie holding a signed token. The password is never stored in the browser, and the game server's admin password never reaches the browser at all. Palhelm makes every game call server-side.

:::note
Behind a TLS-terminating proxy, set `PALHELM_SECURE_COOKIES: "true"` so the session cookie carries the `Secure` flag. Direct TLS and trusted forwarded HTTPS are detected automatically.
:::

## The two roles

Palhelm has two roles: `admin` and `viewer`.

- **Admin** has full control. Admins can run RCON, kick, ban, unban, take and restore backups, edit config, run the graceful shutdown countdown, and manage Integration API keys.
- **Viewer** is read-only. Viewers can see the dashboard, players, the live map, metrics history, and the event feed. They cannot run destructive actions.

Every action that changes something is gated server-side, not just hidden in the UI. A viewer session is rejected by the server if it tries a mutating call, so the role is a real boundary, not a cosmetic one. The UI also adapts: viewers do not see destructive buttons, and destructive entries are hidden from the command palette for viewers.

## Enable the viewer role

The viewer login is optional and off by default. To turn it on, set `PALHELM_VIEWER_PASSWORD` to a strong value and recreate the container:

```yaml
    environment:
      PALHELM_ADMIN_PASSWORD: "choose-a-strong-password"
      PALHELM_VIEWER_PASSWORD: "choose-a-different-strong-password"
```

```sh
docker compose -f ./compose/docker-compose.yml up -d palhelm
```

Give the viewer password to people who should watch the server but not change it. If you leave `PALHELM_VIEWER_PASSWORD` unset, there is no viewer account and only the admin can log in.

Use two different passwords for the two roles. Anyone with the admin password has full control.

## Integration API keys are separate

The read-only Integration API used by the Discord bot and by scripts does not use these logins. It uses bearer-token keys you create as admin under Settings. Those keys are even more restricted than a viewer. See the [Integration API](/integration-api/keys-and-redaction/) section for the key lifecycle and the redaction model.

## Next steps

- [Fetch map tiles and Pal icons](/getting-started/map-tiles-and-icons/) so the live map and Pal art show up.
- [Keep Palhelm up to date](/getting-started/updating/).
