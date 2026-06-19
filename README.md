# Zatabox SDKs

Official client libraries for the [Zatabox Tickets](https://zatabox.com) REST API 
the white-label event-ticketing platform in **Node.js, Python, Ruby, PHP and Go**.

Every SDK covers **all 78 publicly documented REST endpoints** (the
integrator-facing surface published at <https://zatabox.com/docs/api>) with the same
ergonomics: scoped-key / JWT auth with automatic sandbox routing, idempotent writes,
a typed error envelope, cursor auto-pagination, live-stream (SSE) URLs and
inbound-webhook signature verification using **only each language's standard
library** (zero third-party runtime dependencies).

> **Public surface only.** These SDKs deliberately expose *only* what is documented
> for integrators. Internal/platform surfaces platform administration, white-label
> internals, scanner tokens, wallets, MCP tokens, media upload, etc. are not
> included.

| Language | Folder | Runtime deps | Tests |
|---|---|---|---|
| Node.js | [`node/`](./node) | none (`fetch` + `node:crypto`) | ✅ 17 |
| Python | [`python/`](./python) | none (`urllib` + `hmac`) | ✅ 16 |
| Ruby | [`ruby/`](./ruby) | none (`net/http` + `openssl`) | ✅ 16 |
| PHP | [`php/`](./php) | none (`curl`/`json`/`hash` ext) | ✅ 28 (live server) |
| Go | [`go/`](./go) | none (`net/http` + `crypto`) | ✅ 13 |

## Installation

These SDKs are **distributed via this GitHub repository** none is published to npm,
PyPI, RubyGems, Packagist or a third-party Go registry. Each lives in a top-level
language folder. Install the one you need; see that SDK's README for all options.

| Language | Install (from `mysmartrobot/zataboxtickets`) |
|---|---|
| **Python** | `pip install "git+https://github.com/mysmartrobot/zataboxtickets.git#subdirectory=python"` |
| **Go** | `go get github.com/mysmartrobot/zataboxtickets/go@latest` |
| **Ruby** | Gemfile → `gem "zatabox", git: "https://github.com/mysmartrobot/zataboxtickets.git", glob: "ruby/*.gemspec"` |
| **Node.js** | `git clone` the repo, then `npm install ./zataboxtickets/node` ([details](./node#installation)) |
| **PHP** | Composer `path` repository or `require php/src/autoload.php` ([details](./php#installation)) |

## Architecture generated, never drifting

The thing that makes five SDKs *consistent* is that none of the per-endpoint code is
written by hand. There is one source of truth:

```
spec/endpoints.json   ← canonical manifest: every endpoint (ns, method, path, kind)
spec/CONVENTIONS.md   ← the cross-language contract every core implements
scripts/generate.mjs  ← reads the manifest, emits the resource layer for all 5 SDKs
```

```
                      ┌─────────────────────┐
   Zatabox API     ──► │  spec/endpoints.json │ ──► scripts/generate.mjs ──┐
   route definitions  └─────────────────────┘                            │
                                                                          ▼
        node/src/resources.generated.js (+ .d.ts)   python/zatabox/resources.py
        ruby/lib/zatabox/resources.rb                php/src/Resources.php
        go/resources_gen.go
                                                                          │
                          hand-written runtime cores (transport, auth,    │
                          retries, errors, pagination, webhooks, upload) ◄┘
```

Each language pairs a **generated resource layer** (one method per endpoint) with a
small **hand-written core** that implements the shared contract idiomatically. The
manifest is hand-derived from the Zatabox API's own route definitions (the published
OpenAPI spec is intentionally partial), so coverage is exhaustive.

### Regenerate after an API change

```bash
node scripts/generate.mjs
```

The generator validates the manifest (no duplicate routes/methods, well-formed
paths) before emitting, then rewrites every SDK's resource layer. Cores and tests
are untouched.

## The 78 endpoints, by namespace

`auth` · `events` · `organizer` · `eventCustomization` · `tickets` · `orders` ·
`payments` · `checkin` · `community` · `growth` · `users` · `integrations` ·
`webhooks`

(These are exactly the integrator-, organizer- and buyer-facing surfaces published
on the public docs. Namespaces are spelled idiomatically per language 
`eventCustomization` in Node/PHP, `event_customization` in Python/Ruby,
`EventCustomization` in Go.)

## Sandbox / test mode

Zatabox ships a full **sandbox** that mirrors every endpoint with no real money or
email/SMS perfect for development and CI.

- **Sandbox API** `https://sandbox.zatabox.com` (same `/api/v1` surface as
  production). Any **`vt_test_`** key auto-routes here, so you usually don't set a
  base URL at all just construct the client with a test key.
- **Sandbox console** `https://tester.zatabox.com`. Sign in with your production
  Zatabox account to mint/rotate `vt_test_` keys, watch **live request logs**, see
  usage metrics, and browse the mirrored endpoint catalog.
- **Environment fencing** a `vt_test_` key only works against the sandbox and a
  `vt_live_` key only against production; crossing them returns `403 WRONG_ENV`.
- **Self-hosting** a local sandbox backend listens on `:4100`; point an SDK at it
  with an explicit base URL (e.g. `http://localhost:4100`).

```
ZATABOX_API_KEY=vt_test_…   → sandbox (sandbox.zatabox.com)
ZATABOX_API_KEY=vt_live_…   → production (api.zatabox.com)
```

## Shared behaviour (see [`spec/CONVENTIONS.md`](./spec/CONVENTIONS.md))

- **Base URLs** `api.zatabox.com` (live), `sandbox.zatabox.com` (test). A
  `vt_test_` key auto-routes to the sandbox; an explicit base URL always wins.
- **Auth** one `Authorization: Bearer <token>` header: a `vt_live_`/`vt_test_` API
  key, a portal JWT, or a `vt_mcp_` token.
- **Idempotency** every write sends an `Idempotency-Key` (auto UUIDv4 unless you
  pass one).
- **Envelope** success returns the unwrapped `data`; any non-2xx raises a
  `ZataboxError`/`Error` with `code`, `status`, `requestId`, `details`.
- **Pagination** `paginate(listMethod, query)` follows the cursor across pages.
- **Retries** `5xx`/network/timeout retried with exponential backoff; `4xx` never.
- **SSE** `checkin.live` returns a fully-qualified stream URL for an EventSource client.
- **Webhooks** `webhooks.verify(rawBody, signatureHeader, secret)` validates the
  `t=…,v1=…` HMAC-SHA256 signature in constant time.

## Running the test suites

```bash
node scripts/generate.mjs                                  # (re)generate resources
cd node   && npm test
cd python && python3 -m unittest discover -s tests
cd ruby   && ruby -Ilib test/test_client.rb
cd go     && go test ./...
cd php    && php -S 127.0.0.1:8799 tests/server.php & \
             ZBX_TEST_BASE=http://127.0.0.1:8799 php tests/test_client.php
```

## License

MIT
