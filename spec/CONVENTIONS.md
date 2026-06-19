# Zatabox SDK cross-language conventions

This is the contract every Zatabox SDK implements identically. The resource layer
of each SDK is **generated** from [`endpoints.json`](./endpoints.json) by
[`../scripts/generate.mjs`](../scripts/generate.mjs); the runtime *core* of each SDK
is hand-written to the rules below. Keeping the cores aligned to this document is
what makes Node, Python, Ruby, PHP and Go behave the same.

## 1. Transport

- **Base URLs** `https://api.zatabox.com` (live), `https://sandbox.zatabox.com`
  (test / sandbox mirrors the full `/api/v1` surface), `http://localhost:4100`
  (a self-hosted sandbox backend). Every path is prefixed `/api/v1` (already baked
  into the manifest paths). The sandbox developer console is a separate site at
  `https://tester.zatabox.com` (sign in with your production account to mint/rotate
  `vt_test_` keys, inspect live request logs and usage, and browse the endpoint
  catalog).
- **Base-URL resolution** if the caller passes an explicit `baseUrl`, use it.
  Otherwise infer from the API key prefix: `vt_test_…` / `sk_test_…` → sandbox,
  everything else → live.
- **Auth** a single `Authorization: Bearer <token>` header. The token is an API
  key (`vt_live_…` / `vt_test_…`), a portal JWT access token, or an MCP token
  (`vt_mcp_…`). The SDK does not care which; it forwards whatever credential was
  configured. A bearer token can be swapped at runtime (after a refresh).
- **Headers on every request** `Accept: application/json`, `User-Agent:
  zatabox-<lang>/<version>`, and `Content-Type: application/json` whenever a body
  is sent.

## 2. Request shape

- Path parameters (`:id`, `:orgId`, …) are **positional** method arguments in
  declaration order, each URL-encoded.
- `GET`/`DELETE`-style reads take an optional **query** map as the next argument.
- `POST`/`PUT`/`PATCH`/`DELETE` writes take an optional **body** value (serialized
  as JSON) as the next argument.
- A trailing **options** bag accepts `idempotencyKey`, extra `headers`, an extra
  `query`, and a per-call `raw` flag.

## 3. Idempotency

Every write (`POST`/`PUT`/`PATCH`/`DELETE`) sends an `Idempotency-Key` header. If
the caller supplies one it is used verbatim; otherwise the SDK generates a UUIDv4.
A fresh key is generated per *logical* call, so two independent `.create()` calls
produce two records pass your own key when you want dedupe. (The server caches a
keyed response for 24h; replaying the same key + body returns the original result,
same key + a different body → `409 IDEMPOTENCY_KEY_REUSED`.)

## 4. Response envelope

Success: `{ "success": true, "data": <payload>, "meta": { "request_id", "timestamp" } }`.
The SDK **unwraps and returns `data`**. JSON-typed languages return the raw `data`
document for the caller to decode.

Error: `{ "success": false, "error": { "code", "message", "details"? }, "meta": { "request_id" } }`.
On any non-2xx the SDK raises/returns a **`ZataboxError`** carrying `code` (stable
machine string), `message`, `status` (HTTP), `requestId`, and `details`.

## 5. Pagination

Lists are cursor-paginated, but the cursor lives in different places depending on
the endpoint (`data.pagination.cursor` + `has_more`, or `data.nextCursor`). Each
SDK ships an `autoPaginate`/`paginate` helper that calls a list method repeatedly,
reading the next cursor from any of those shapes and stopping when it is null.

## 6. Retries & timeouts

- Default timeout: 30s (configurable).
- Transient failures (`5xx`, network errors, timeouts) are retried with exponential
  backoff, up to `maxRetries` (default 2). `4xx` responses are never retried.
- `429` is surfaced as a `ZataboxError` (code `RATE_LIMITED`); callers can read
  `Retry-After` from `error.details` when present.

## 7. Special endpoint kinds (`kind` in the manifest)

- `binary` PDF/CSV/XML byte streams (`tickets.pdf`, `orders.invoice`,
  `checkin.export`, `organizer.eventExport`, `site.sitemap`). The method returns
  raw bytes (and content type) instead of a parsed envelope.
- `sse` `checkin.live` is a Server-Sent-Events stream. Generated as a
  `*Url(...)` helper that returns the fully-qualified URL (with auth handled by the
  caller's chosen SSE client).
- `multipart` `media.upload` is `multipart/form-data`; it is hand-written in each
  core rather than generated.

## 8. Webhooks (inbound)

`client.webhooks.verify(payload, signatureHeader, secret)` validates an inbound
webhook signature. Scheme: header `t=<unix>,v1=<hex-hmac-sha256>`; the signed string
is `<t>.<rawBody>`, HMAC-SHA256 with the endpoint secret, compared in constant time.
Returns the parsed JSON event on success, raises `ZataboxError` otherwise.

## 9. Naming (idiomatic per language, identical structure)

| | namespaces / methods | classes |
|---|---|---|
| Node | `camelCase` | |
| Python | `snake_case` | `PascalCase` |
| Ruby | `snake_case` | `PascalCase` |
| PHP | `camelCase` | `PascalCase` |
| Go | `PascalCase` (exported) | `PascalCase` |

The generator converts the manifest's canonical `camelCase` names into each
language's idiom, so `client.savedSearches` (Node) is `client.saved_searches`
(Python/Ruby), `$client->savedSearches` (PHP) and `client.SavedSearches` (Go).

## 10. Zero runtime dependencies

Each SDK uses only its language's standard library: Node global `fetch` +
`node:crypto`; Python `urllib` + `hmac`/`hashlib`; Ruby `net/http` + `openssl`;
PHP the bundled `curl`/`hash`/`json` extensions; Go `net/http` + `crypto/hmac`.
