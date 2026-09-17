# Credential-isolation sidecar (M4)

Run the Shoplazza CLI in an **untrusted / multi-tenant sandbox** without ever putting a real
merchant token inside that sandbox. The sandbox CLI holds only **sentinel** placeholder tokens;
its API requests are HMAC-signed and rerouted to this sidecar, which runs on a **trusted host**,
injects the real token, and forwards to the Shoplazza API.

```
sandbox (untrusted)                 trusted host                     Shoplazza API
  CLI (-tags authsidecar)  --HMAC-->  shoplazza-sidecar  --real token-->  https://<store>
  sentinel token only                 verifies + injects
```

This is not a command RPC gateway — it is a **credential proxy**. The sandbox still runs the CLI
(shell); only the credentials are isolated.

## Why

A hosted platform that runs the CLI for many merchants must not inject each merchant's real
access token into the sandbox process: a sandbox escape or a cross-tenant bug would leak it. The
sidecar keeps real tokens on the trusted host and hands the sandbox only sentinels.

## Security properties

- **HMAC-SHA256** over a canonical request (method, host, path+query, body hash, timestamp,
  identity, auth-header). Any tampering invalidates it.
- **Replay window** ±60s (timestamp is signed).
- **Token-injection allowlist**: the real token is only ever written into `Access-Token`,
  `Cli-Partner-Token`, or `Authorization` — never smuggled into another header.
- **Host allowlist / https pinning**: requests may only target the configured store host(s),
  over https (SSRF / downgrade guard).
- **Multi-tenant isolation**: per-client keys; the key that verifies a request identifies the
  client, and only that client's own token is resolved — no cross-tenant fallback.
- **Fail-closed CLI**: a CLI built WITHOUT `-tags authsidecar` refuses to start if
  `SHOPLAZZA_CLI_AUTH_PROXY` is set (exit 2), so a misbuilt binary can't bypass isolation.
- **Audit**: every request logged with method / host / client / identity / status / duration;
  path ids and query strings are redacted.

## Build

```bash
# The sidecar server (trusted host):
go build -o shoplazza-sidecar ./sidecar/server

# The sandbox CLI MUST be built with the tag so the interceptor is compiled in:
go build -tags authsidecar -o shoplazza .
```

## Run — single-tenant

Trusted host (already logged in with `shoplazza auth login`):

```bash
./shoplazza-sidecar --listen 127.0.0.1:16384
# prints the exact env to set in the sandbox, including the generated HMAC key
```

Sandbox:

```bash
export SHOPLAZZA_CLI_AUTH_PROXY="http://127.0.0.1:16384"
export SHOPLAZZA_CLI_PROXY_KEY="<key from the sidecar>"
export SHOPLAZZA_ACCESS_TOKEN="sidecar-managed-store"      # the store sentinel
export SHOPLAZZA_CLI_API_BASE_URL="https://<store-domain>"
shoplazza orders +search --financial-status paid          # runs with zero real credentials
```

## Run — multi-tenant

Each client acts as its own CLI **profile** (the profile == the client name).

```bash
# 1. on the trusted host, log in once per store:
shoplazza auth login          # creates profile "acme"
shoplazza auth login          # creates profile "globex"

# 2. write a 32-byte hex key per client:
mkdir -p ~/.shoplazza-sidecar/keys
openssl rand -hex 32 > ~/.shoplazza-sidecar/keys/acme.key
openssl rand -hex 32 > ~/.shoplazza-sidecar/keys/globex.key
chmod 600 ~/.shoplazza-sidecar/keys/*.key

# 3. run in multi-tenant mode:
./shoplazza-sidecar --listen 0.0.0.0:16384 --keys-dir ~/.shoplazza-sidecar/keys
```

Give each client **only its own** key + this sidecar URL. A request signed with `acme.key` can
only resolve `acme`'s token and only reach `acme`'s store.

## Container

See [`Dockerfile`](./Dockerfile). Real credentials are mounted at runtime, never baked in.

## Status / scope

- **P1 single-tenant** and **P2 multi-tenant (pre-provisioned profiles)**: implemented.
- **Not yet**: OAuth device-flow self-service onboarding endpoints (Lark's `/_sidecar/auth/*`) —
  the pre-provisioning model above covers isolation without them. Add them if self-service client
  onboarding is needed.
