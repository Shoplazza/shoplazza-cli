# Credential-isolation sidecar — reference implementation (demo)

**This is reference code, not a shipped product.** Like the Lark CLI's `server-demo`, it is a
working example of the trusted-host side of the credential-isolation sidecar. Copy it into your
platform and adapt it — do not deploy it as-is. Your platform owns the parts that can't be
generic: where real credentials live, how clients are provisioned/authorized, and how the
service is deployed, scaled, and monitored.

The reusable, stable pieces are the **wire protocol and HMAC** in `internal/sidecar`
(`protocol.go`, `hmac.go`) and the client-side interceptor — those are meant to be used as-is.
This server is the adaptable part.

## What it demonstrates

Run the CLI in an untrusted/multi-tenant sandbox without a real merchant token ever entering it.
The sandbox holds only sentinel tokens; its API requests are HMAC-signed by the interceptor
(compiled in with `-tags authsidecar`) and rerouted here, where the real token is injected before
forwarding to the Shoplazza API.

```
sandbox (untrusted)                trusted host                    Shoplazza API
  CLI -tags authsidecar  --HMAC-->  this reference server  --real token-->  https://<store>
  sentinel token only               verifies + injects
```

## Security properties (the contract to preserve when you adapt it)

- HMAC-SHA256 over a canonical request (method/host/path+query/body-hash/timestamp/identity/
  auth-header); ±60s replay window.
- Real token injected only into `Access-Token` / `Cli-Partner-Token` / `Authorization` (no
  smuggling into other headers); target-host allowlist + https pinning (SSRF/downgrade guard).
- Multi-tenant: per-client keys; the verifying key identifies the client, and only that client's
  own token is resolved — no cross-tenant fallback.
- The standard CLI (built WITHOUT `-tags authsidecar`) fails closed (exit 2) if
  `SHOPLAZZA_CLI_AUTH_PROXY` is set.
- Audit log redacts path ids + query strings.

## Try it locally

```bash
# reference server (from repo root; on a host already logged in via 'shoplazza auth login'):
go build -o shoplazza-sidecar ./sidecar/server-demo
./shoplazza-sidecar --listen 127.0.0.1:16384        # prints the sandbox env to set

# the sandbox CLI must be built with the tag so the interceptor is compiled in:
go build -tags authsidecar -o shoplazza .
```

Sandbox env (single-tenant; the server prints these):

```bash
export SHOPLAZZA_CLI_AUTH_PROXY="http://127.0.0.1:16384"
export SHOPLAZZA_CLI_PROXY_KEY="<key from the server>"
export SHOPLAZZA_ACCESS_TOKEN="sidecar-managed-store"     # the store sentinel
export SHOPLAZZA_CLI_API_BASE_URL="https://<store-domain>"
shoplazza orders +search --financial-status paid          # runs with zero real credentials
```

Multi-tenant (`--keys-dir`): each client acts as its own CLI profile; write a per-client
`<profile>.key` and hand each client only its own key. The server prints the specifics.

## What you must replace when adapting

- **Credential source** (`resolver.go` / `MultiResolver`): this demo reads the trusted host's own
  CLI keychain/profiles. Point it at your platform's credential store.
- **Client provisioning / authorization**: this demo uses pre-provisioned profiles + key files.
  If you need self-service onboarding, add your own auth flow (Lark's demo adds OAuth device-flow
  management endpoints — not ported here).
- **Deployment / scaling / monitoring**: your platform's concern, not this reference.
