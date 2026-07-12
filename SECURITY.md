# Security policy

## Deployment guidance: keep the panel off the open internet

Palhelm is an admin tool. It holds your game server's admin password and can restore saves, kick and ban players, and edit server config. **Never expose the panel to the open internet.** Bind it to localhost, a LAN, or a VPN/tailnet interface only, for example:

```yaml
    ports:
      - "127.0.0.1:8080:8080"
```

This is the same advice Pocketpair gives for the game's own REST API. If you must reach the panel remotely, do it over a VPN or tailnet, not a public port. If you terminate TLS in front of it, set `PALHELM_TRUSTED_PROXIES` to your proxy's CIDR and consider `PALHELM_SECURE_COOKIES=true`.

Other habits worth keeping:

- Use a strong, unique `PALHELM_ADMIN_PASSWORD`. It is not the same as the game admin password and should not be.
- Treat Integration API keys (`phk_` prefix) like passwords. They are read-only and redacted by design, but revoke any key you think leaked; revocation takes effect on the next request.
- The panel deliberately does not need or accept a Docker socket mount. Do not add one.

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub security advisories on this repo: go to the **Security** tab at [github.com/8tp/palhelm](https://github.com/8tp/palhelm/security) and choose **Report a vulnerability**. Do not open a public issue or PR for a security problem.

Include what you can: affected version, setup, steps to reproduce, and impact. You will get a response as soon as maintainer time allows; this is a volunteer project, so please allow a reasonable window for a fix before any public disclosure.
