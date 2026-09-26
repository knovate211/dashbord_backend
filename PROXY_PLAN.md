# Reverse and forward proxy — implementation plan

Right now the api-gateway is the front door. It publishes `8080:8080` straight
to the host, speaks plain HTTP, and nothing sits between it and a client. On the
way out, it calls Razorpay, RapidAPI (JSearch) and an SMTP server directly, and
nothing limits where else it could call.

This plan adds two proxies:

- A **reverse proxy (Caddy)** for inbound traffic. It terminates HTTPS, is the
  only thing that can reach the gateway, and tells the gateway who the real
  client is.
- A **forward proxy (Squid)** for outbound traffic. The gateway may reach only
  an allowlist of external hosts, and every outbound call is logged.

The stack only runs locally today, so every phase is built and proven on the
local compose stack first. Production-only steps are kept separate in
[Phase 5](#phase-5--production-later) so they can be picked up when a server
exists.

Do the phases in order. Phase 1 fixes a bug that exists today, even without any
proxy.

---

## Target design

```
                          INBOUND
  Browser / frontend ──HTTPS──► Caddy ──http──► api-gateway ──gRPC──► internal services
                                  │                  │
                                  │  /ws ────────────┴──ws──► notification-service
                                  │
                        the only public port

                          OUTBOUND
  api-gateway ──HTTPS_PROXY──► Squid ──► api.razorpay.com
                                  │  ──► jsearch.p.rapidapi.com
                                  │   ✗  anything else: denied and logged
  api-gateway ──SMTP─────────► mailpit (local) / mail provider (prod)
  execution sandbox ─────────► nothing (already NetworkMode: "none")
```

What each piece is responsible for:

| Concern | Owner |
|---|---|
| TLS certificates and HTTPS | Caddy |
| Request body size, timeouts, security headers | Caddy |
| Real client IP (`X-Real-IP`) | Caddy sets it, the gateway trusts only that |
| Auth, CORS, per-route rate limiting | api-gateway (unchanged) |
| Routing `/ws` to notification-service | api-gateway (unchanged) |
| Which external hosts may be called | Squid allowlist |
| Outbound call audit log | Squid `access.log` |

### Current outbound traffic

Only the api-gateway talks to the internet. The other services only call each
other, Postgres, Redis and NATS.

| Destination | Protocol | Code |
|---|---|---|
| `api.razorpay.com` | HTTPS | `graph/resolvers/enroll.go`, `certification.go` |
| `jsearch.p.rapidapi.com` | HTTPS | `graph/resolvers/job.resolvers.go` |
| SMTP (`SMTP_HOST:SMTP_PORT`) | raw TCP | `*_notify.go` via `net/smtp` |

The code-execution sandbox already runs with `NetworkMode: "none"`
(`services/execution-service/internal/sandbox/docker.go`). Keep it that way. It
must never be given a route through Squid.

---

## Phase 1 — Fix client IP detection (code)

**Why first:** `clientIP()` in `services/api-gateway/graph/resolvers/inquiry.go`
returns the leftmost `X-Forwarded-For` value. The client controls that header,
so anyone can send a different fake IP on every request and bypass every
IP-based limit:

- inquiry, enroll, password reset
- scholarship apply and claim
- certification apply and claim
- hiring claim
- integrity logging

This bug exists today. Once a proxy is added, the gateway also needs to know
that the proxy's header is the one to trust.

**Change:**

1. Add an env var `TRUST_PROXY_HEADERS` (default `false`).
2. Rewrite `clientIP()`:
   - If `TRUST_PROXY_HEADERS=true` and `X-Real-IP` is set, return it. Caddy
     always overwrites this header, so a client can't forge it.
   - Otherwise return the host part of `r.RemoteAddr`.
   - Never read `X-Forwarded-For`.
3. Read the flag once at startup. Don't call `os.Getenv` per request.
4. Add `clientip_test.go` covering:
   - spoofed `X-Forwarded-For` is ignored in both modes
   - `X-Real-IP` is ignored when the flag is off
   - `X-Real-IP` is used when the flag is on
   - `RemoteAddr` with and without a port

**Done when:** `go test ./services/api-gateway/...` passes, and this returns 429
after the limit even though the header changes every time:

```bash
for i in $(seq 1 30); do
  curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost:8080/api/inquiry \
    -H "X-Forwarded-For: 10.0.0.$i" -H "Content-Type: application/json" -d '{}'
done
```

Adjust the path and body to the real inquiry route.

**Risk:** none for local dev. Without the flag the behaviour is `RemoteAddr`,
which is correct for direct access.

---

## Phase 2 — Reverse proxy (Caddy), local

Add Caddy in front of the gateway **without removing `8080` yet**. That way the
frontends keep working while HTTPS is tested side by side.

### Files

`deployments/caddy/Caddyfile`

```
{
    # Local only: Caddy's own CA issues the cert for localhost.
    local_certs
}

localhost:8443 {
    encode gzip

    request_body {
        max_size 10MB
    }

    header {
        Strict-Transport-Security "max-age=31536000"
        X-Content-Type-Options    "nosniff"
        Referrer-Policy           "strict-origin-when-cross-origin"
        -Server
    }

    reverse_proxy api-gateway:8080 {
        header_up X-Real-IP {remote_host}
        # WebSocket upgrades for /ws are handled automatically.
        transport http {
            read_timeout  120s
            write_timeout 120s
        }
    }

    log {
        output stdout
        format json
    }
}
```

`docker-compose.yml`, new service:

```yaml
  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports:
      - "8443:8443"
    volumes:
      - ./deployments/caddy/Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      - api-gateway
```

Also add `caddy_data:` and `caddy_config:` under `volumes:`, and
`TRUST_PROXY_HEADERS: "true"` to the api-gateway environment.

> **Local caveat:** with `TRUST_PROXY_HEADERS=true` and `8080` still published,
> a client hitting `:8080` directly could send its own `X-Real-IP`. That's
> acceptable locally, and it's exactly why Phase 5 removes the published port.

### Trust the local certificate (once per machine)

```bash
docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt ./caddy-local-root.crt
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain ./caddy-local-root.crt
rm ./caddy-local-root.crt
```

### Frontends

Add `https://localhost:8443` to the frontends' API base URL as an option. Point
at it when testing through the proxy, and leave `http://localhost:8080` as the
default. `ALLOWED_ORIGINS` doesn't change: CORS depends on the frontend's
origin, not the API's.

### Done when

- `curl https://localhost:8443/api/health` returns `{"status":"ok"}` with no
  `-k` flag.
- Login works through `https://localhost:8443/login`.
- A WebSocket to `wss://localhost:8443/ws` connects and receives a
  notification.
- The response headers include `Strict-Transport-Security` and no `Server`
  header.
- An upload larger than 10MB returns 413 from Caddy.
- The gateway logs show the real client IP. Test it: sending
  `X-Real-IP: 1.2.3.4` through Caddy is overwritten.

**Rollback:** `docker compose rm -sf caddy`. Port 8080 still works.

---

## Phase 3 — Forward proxy (Squid), local

### Files

`deployments/squid/squid.conf`

```
http_port 3128

# Hosts the gateway may reach. Add new integrations here, deliberately.
acl allowed_hosts dstdomain api.razorpay.com
acl allowed_hosts dstdomain jsearch.p.rapidapi.com

acl SSL_ports port 443
acl CONNECT method CONNECT

http_access deny CONNECT !SSL_ports
http_access allow allowed_hosts
http_access deny all

# Tunnels only, never cache.
cache deny all

access_log stdio:/dev/stdout
cache_log  stdio:/dev/stderr
```

`docker-compose.yml`, new service:

```yaml
  squid:
    image: ubuntu/squid:latest
    restart: unless-stopped
    volumes:
      - ./deployments/squid/squid.conf:/etc/squid/squid.conf:ro
    # No ports: only reachable on the compose network.
```

api-gateway environment additions:

```yaml
      HTTPS_PROXY: http://squid:3128
      NO_PROXY: localhost,127.0.0.1,problem-service,submission-service,execution-service,progress-service,user-service,assessment-service,notification-service,postgres,redis,nats,mailpit
```

Add `squid` to the gateway's `depends_on`.

### Why `NO_PROXY` is mandatory

- **gRPC:** grpc-go reads `HTTPS_PROXY` too. Without `NO_PROXY`, every call to
  the internal services tries to tunnel through Squid, and Squid refuses because
  ports 5005x aren't in `SSL_ports`. The gateway then fails at startup or on the
  first request.
- **HTTP clients:** no Go code changes are needed. The Razorpay and JSearch
  clients are `&http.Client{Timeout: ...}` with no custom `Transport`, so they
  use `http.DefaultTransport`, which already honours `HTTPS_PROXY`.
- **Keep it that way:** if anyone later sets a custom `Transport`, it must
  include `Proxy: http.ProxyFromEnvironment`, or it silently bypasses Squid.
  Add a comment next to each client saying so.

### SMTP

`net/smtp` opens a raw TCP connection and ignores `HTTPS_PROXY`. Locally it goes
to mailpit on the compose network, which is fine. Production handling is in
Phase 5.

### Done when

- The gateway starts and all gRPC-backed pages work. That proves `NO_PROXY` is
  right.
- The jobs search (JSearch) returns results, and Squid's log shows
  `CONNECT jsearch.p.rapidapi.com:443 ... TCP_TUNNEL/200`.
- A Razorpay order is created (test keys), and Squid logs the CONNECT to
  `api.razorpay.com`.
- A blocked host is refused:

  ```bash
  docker compose exec api-gateway wget -qO- https://example.com
  # expect failure: Squid returns 403 on CONNECT
  ```

  If the image has no `wget`, run the same check from a throwaway container on
  the same network with `-e HTTPS_PROXY=http://squid:3128`.
- Emails still arrive in mailpit (`http://localhost:8025`).

**Rollback:** remove `HTTPS_PROXY` and `NO_PROXY` from the gateway and restart
it.

---

## Phase 4 — Network isolation (prod compose only)

A proxy only protects you if services can't go around it. Docker's
`internal: true` networks have no route to the internet. That's what forces
egress through Squid.

**Don't do this in the local `docker-compose.yml`.** Containers on an
internal-only network can't publish ports. Locally you want 5432, 6380, 8025
and the gRPC ports reachable from your machine. Put the isolation in
`docker-compose.prod.yml`, and test it locally with both files:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

```yaml
networks:
  edge:            # Caddy's public side
  backend:
    internal: true # no internet route
  egress:          # Squid's way out

services:
  caddy:
    networks: [edge, backend]
  squid:
    networks: [backend, egress]
  api-gateway:
    networks: [backend]
    ports: []      # only Caddy can reach it
  # every other service, postgres, redis and nats:
  #   networks: [backend]
```

**Done when:**

- `curl http://localhost:8080` fails (nothing is listening).
- `https://localhost:8443` works end to end.
- Allowed external calls still succeed through Squid.
- From inside the gateway, a direct call with the proxy unset fails:

  ```bash
  docker compose exec -e HTTPS_PROXY= api-gateway wget -qO- https://api.razorpay.com
  ```

---

## Phase 5 — Production (later)

Do this when a server exists. Everything above carries over. These are the
differences:

1. **Real domain and certificate.** Point `api.<domain>` at the server. In the
   Caddyfile, replace `localhost:8443` with `api.<domain>` and remove
   `local_certs`. Caddy then gets and renews Let's Encrypt certificates on its
   own. Publish `80:80` and `443:443`. Port 80 is needed for the certificate
   challenge and the HTTP→HTTPS redirect.
2. **Remove `8080:8080`** (already done by the Phase 4 prod override). Also
   close 8080 in the security group or firewall.
3. **Frontend.** Set the production API base URL to `https://api.<domain>`.
   The CloudFront frontend can then call the API without mixed-content errors.
4. **SMTP egress.** With the gateway on an internal-only network, SMTP to a real
   provider is blocked. Pick one:
   - **Recommended:** switch to the mail provider's HTTPS API, and add its host
     to the Squid allowlist. All egress then goes through one audited path.
   - Put a small SMTP relay on `backend` + `egress`, and point `SMTP_HOST` at it.
5. **Squid logs.** Ship `access.log` to wherever logs go, and alert on
   `TCP_DENIED`. A denied CONNECT from the gateway means either a new
   integration someone forgot to allowlist, or something that shouldn't be
   happening.
6. **Kubernetes, if you move there.** The files in `deployments/k8s/` currently
   expose the gateway as a `LoadBalancer` on plain port 80. Equivalent setup:
   - ingress-nginx or Traefik, plus cert-manager, instead of Caddy
   - change the gateway Service to `ClusterIP`
   - run Squid as a Deployment and Service
   - add a default-deny egress `NetworkPolicy` that allows only Squid, DNS and
     in-cluster services

---

## Checklist

- [ ] Phase 1: `clientIP()` ignores `X-Forwarded-For`, trusts `X-Real-IP` only
      behind the flag, with tests
- [ ] Phase 2: Caddy on `https://localhost:8443`; health, login, `/ws` and
      body limit verified
- [ ] Phase 3: Squid allowlist; JSearch and Razorpay pass, `example.com`
      blocked, gRPC unaffected
- [ ] Phase 4: prod override isolates networks; `:8080` closed, direct egress
      fails
- [ ] Phase 5: domain, real certificate, SMTP decision, log alerting

## Open decisions

- **API domain name:** needed for Phase 5.
- **Hosting target:** a single server with compose, or Kubernetes. This decides
  Caddy vs. ingress-nginx.
- **SMTP in production:** HTTPS mail API or SMTP relay.
- **Global rate limiting at Caddy:** needs a Caddy build with a rate-limit
  plugin. Deferred: the gateway's per-route limiters cover the sensitive
  endpoints once Phase 1 makes them trustworthy.
