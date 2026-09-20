# Temporary proxy TTL (auto-expire tunnels)

For short-lived access (e.g. a tunnel opened for an external contractor), a
proxy can carry a `ttl`. When the lifetime is over, frps closes the tunnel
itself and keeps rejecting registrations under the same proxy name, so a
forgotten `frpc` cannot silently revive the tunnel by reconnecting.

Proxies without `ttl` keep the existing behavior.

## Configuration (frpc)

```toml
[[proxies]]
name = "contractor-ssh"
type = "tcp"
localIP = "127.0.0.1"
localPort = 22
remotePort = 6001
ttl = "2h"
```

`ttl` accepts any Go duration (`"90m"`, `"2h"`, `"30s"`, ...) of at least 1
second. An empty, zero or illegal value makes frpc fail to load the
configuration; frps also re-validates the value on every registration.

Legacy INI format (`frpc.ini`) supports the same key:

```ini
[contractor-ssh]
type = tcp
local_ip = 127.0.0.1
local_port = 22
remote_port = 6001
ttl = 2h
```

## Semantics

- The deadline is fixed when the proxy first registers. Reconnecting before
  expiry (network blip, frpc restart) never extends the TTL.
- At expiry, frps:
  1. stops listening on the remote port and removes the proxy;
  2. pushes `CloseProxy` to frpc, which marks the proxy `closed by server`
     instead of retrying;
  3. logs a warning:
     `proxy [name] ttl expired at <time> (requested ttl 2h0m0s), tearing down temporary tunnel`.
- After expiry the proxy name is blocked. A new registration (even from a
  freshly started frpc with the same config) fails with:
  `proxy [name] is expired at <time>, re-registration is denied until the TTL grant is cleared`.
- The ledger is in-memory: restarting frps clears it. To grant access again,
  use a new proxy name or restart frps.

## Observability

The dashboard admin API exposes the TTL state on both v1 and v2 proxy
endpoints:

```
GET /api/proxies/{type}/{name}
GET /api/proxies/{name}
GET /api/v2/proxies/{name}
GET /api/v2/proxies
```

Response fields:

- `ttl`: configured lifetime, e.g. `"2h"` (absent for normal proxies);
- `expiresAt`: absolute expiry as a unix timestamp;
- `remainingSeconds`: seconds left, `0` when expired, `-1` without a TTL;
- `expired`: `true` once the grant is expired.

On v2 the proxy `phase` becomes `"expired"`. The frps dashboard proxies list
and detail page show a TTL/remaining indicator and an `expired` badge.

## Reproduce

With frps running, start frpc with a 3 second TTL TCP proxy:

```toml
serverAddr = "127.0.0.1"
serverPort = 7000

[[proxies]]
name = "temp-tcp"
type = "tcp"
localPort = 1024
remotePort = 6001
ttl = "3s"
```

1. `nc 127.0.0.1 6001` works immediately.
2. After 3s the connection port is closed; frps logs the ttl-expired line
   and frpc logs `server requested to close proxy [temp-tcp]`.
3. Restarting the same frpc keeps failing with the `is expired` error, and
   the port stays closed.

This scenario is covered by the e2e regression
`[Feature: TemporaryProxyTTL]` (`test/e2e/v1/features/temporary_proxy_ttl.go`)
and the fake-clock unit tests in `server/ttl/ttl_test.go`.
