# @zatabox/node

Official **Node.js SDK** for the [Zatabox Tickets](https://zatabox.com) REST API the
white-label event-ticketing platform where the box office is an API. This SDK is a
thin, fully-typed client over `https://api.zatabox.com/api/v1` that handles auth,
sandbox routing, idempotency, retries, pagination, live (SSE) streaming and
webhook verification for you.

- **Zero runtime dependencies** built on the global `fetch` and `node:crypto`.
- **Complete** every one of the **78 REST endpoints** is a typed method.
- **Generated, never drifts** the resource layer is emitted from the canonical
  [`endpoints.json`](../spec/endpoints.json) spec.
- **TypeScript-first** ships hand-written + generated `.d.ts` declarations.

> Jump to the [Full endpoint reference](#full-endpoint-reference) for every method.

---

## Table of contents

- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Authentication](#authentication)
- [Sandbox / test mode](#sandbox--test-mode)
- [Client configuration](#client-configuration)
- [How methods map to endpoints](#how-methods-map-to-endpoints)
- [Responses](#responses)
- [Error handling](#error-handling)
- [Idempotency](#idempotency)
- [Retries, timeouts & networking](#retries-timeouts--networking)
- [Pagination](#pagination)
- [Live check-in stream (SSE)](#live-check-in-stream-sse)
- [Verifying inbound webhooks](#verifying-inbound-webhooks)
- [End-to-end recipes](#end-to-end-recipes)
- [TypeScript](#typescript)
- [Concurrency & reuse](#concurrency--reuse)
- [Troubleshooting & FAQ](#troubleshooting--faq)
- [Full endpoint reference](#full-endpoint-reference)
- [Versioning & support](#versioning--support)
- [License](#license)

---

## Requirements

- **Node.js 18 or newer** (the SDK uses the built-in global `fetch`, `Blob`,
  `FormData` and `AbortController`). On Node 16 or below, pass your own `fetch`
  implementation via the `fetch` option.
- A Zatabox **API key** (`vt_live_…` / `vt_test_…`) or a **portal JWT** /
  **MCP token**. Mint API keys from the organizer portal → Integrations, or from the
  [sandbox console](https://tester.zatabox.com).

## Installation

This SDK is **distributed via GitHub** it is not published to npm. It lives in the
`node/` directory of
[`mysmartrobot/zataboxtickets`](https://github.com/mysmartrobot/zataboxtickets).
Because npm cannot install a subdirectory of a git repo directly, use one of:

**Option A clone, then install the local path** (recommended):

```bash
git clone https://github.com/mysmartrobot/zataboxtickets.git
npm install ./zataboxtickets/node
```

This records `"@zatabox/node": "file:./zataboxtickets/node"` in your `package.json`.
Update later with `git -C zataboxtickets pull && npm install ./zataboxtickets/node`.

**Option B git submodule** (pinned and easy to update):

```bash
git submodule add https://github.com/mysmartrobot/zataboxtickets.git vendor/zataboxtickets
npm install ./vendor/zataboxtickets/node
```

**Option C vendor the folder**: copy the repo's `node/` directory into your project
and `require('./vendor/zatabox-node/src')`.

The SDK has **zero dependencies**, so there is nothing else to fetch. Then import it:

```js
// CommonJS
const { ZataboxClient, ZataboxError } = require('@zatabox/node');

// ES modules / TypeScript
import { ZataboxClient, ZataboxError } from '@zatabox/node';
```

## Quick start

```js
const { ZataboxClient } = require('@zatabox/node');

// A vt_test_ key auto-routes to the sandbox; a vt_live_ key to production.
const zatabox = new ZataboxClient({ apiKey: process.env.ZATABOX_API_KEY });

async function main() {
  // 1. Create a draft event
  const event = await zatabox.organizer.createEvent({
    title: 'Warehouse Sessions 004',
    category: 'music',
    startDate: '2026-08-22T20:00:00Z',
    endDate: '2026-08-23T02:00:00Z',
    timezone: 'Africa/Lagos',
    venueType: 'physical',
    venueName: 'Dock 12',
    venueCity: 'Lagos',
    capacity: 450,
    returnPolicy: 'Refunds up to 48h before doors.',   // shown publicly on the event page
    highlightVideoUrl: 'https://youtu.be/dQw4w9WgXcQ',  // embedded highlight player
  });

  // 2. Add a ticket type
  await zatabox.organizer.createTicket(event.id, {
    name: 'General Admission',
    type: 'general',
    price: 5000,
    currency: 'NGN',
    quantityTotal: 450,
    saleStart: '2026-07-01T00:00:00Z',
    saleEnd: '2026-08-22T20:00:00Z',
    transferable: true,                                 // holders can pass it on (false to lock)
    accessUrl: 'https://api.zatabox.com/media/123',     // digital delivery, buyer-only
    accessNote: 'Unzip password: dock12',
  });

  // 3. Publish it
  await zatabox.organizer.publishEvent(event.id);

  // 4. Read it back from the public API
  const live = await zatabox.events.get(event.slug);
  console.log('published:', live.title, '→', live.status);
}

main().catch((err) => {
  console.error(err.code, err.message, err.requestId);
  process.exitCode = 1;
});
```

## Authentication

The SDK forwards a single `Authorization: Bearer <token>` header. You can authenticate
three ways:

```js
// 1. Scoped API key (server-to-server). Prefix decides the environment.
new ZataboxClient({ apiKey: 'vt_live_…' });   // production
new ZataboxClient({ apiKey: 'vt_test_…' });   // sandbox (auto-routed)

// 2. Portal session JWT or MCP token (vt_mcp_…)
new ZataboxClient({ bearerToken: 'eyJhbGciOi…' });

// 3. Explicit base URL (wins over key-prefix routing useful for local dev)
new ZataboxClient({ apiKey: 'vt_test_…', baseUrl: 'http://localhost:4100' });
```

### Passwordless buyer login

Zatabox buyers sign in with a 6-digit email code rather than a password:

```js
const anon = new ZataboxClient({ bearerToken: 'unused', baseUrl: 'https://api.zatabox.com' });
await anon.auth.requestToken({ email: 'fan@example.com' });          // emails a code
const session = await anon.auth.exchangeToken({ email: 'fan@example.com', code: '123456' });
// session = { user, accessToken, refreshToken, tokenType }

const buyer = new ZataboxClient({ bearerToken: session.accessToken });
const tickets = await buyer.users.tickets();
```

### Refreshing & swapping tokens

Access tokens expire; rotate them without rebuilding the client:

```js
const next = await buyer.auth.refresh({ refreshToken: session.refreshToken });
buyer.setBearerToken(next.accessToken);
```

### API-key scopes

API keys can be minted with least-privilege scopes (organizer portal → Integrations).
A key scoped to e.g. `["checkin:write"]` can scan tickets and nothing else. Available
scopes: `events:read`, `events:write`, `tickets:read`, `tickets:write`, `orders:read`,
`orders:write`, `attendees:read`, `attendees:write`, `checkin:write`, `payouts:read`,
`payouts:write`, `webhooks:manage`, `analytics:read`, and `*` (org admins). A call that
exceeds the key's scopes returns `403 INSUFFICIENT_SCOPE`.

## Sandbox / test mode

`vt_test_` keys auto-route to the Zatabox **sandbox** at `https://sandbox.zatabox.com`
a full mirror of the API with no real charges, emails or SMS.

```js
new ZataboxClient({ apiKey: 'vt_test_…' }); // → sandbox.zatabox.com
new ZataboxClient({ apiKey: 'vt_live_…' }); // → api.zatabox.com
new ZataboxClient({ apiKey: 'vt_test_…', baseUrl: 'http://localhost:4100' }); // self-hosted
```

Mint/rotate `vt_test_` keys, watch **live request logs**, see usage and browse the
endpoint catalog in the **sandbox console at https://tester.zatabox.com** (sign in
with your production account). A test key used against production or vice-versa 
returns `403 WRONG_ENV`.

## Client configuration

```js
new ZataboxClient({
  apiKey: 'vt_live_…',     // or bearerToken
  baseUrl: undefined,       // override the auto-resolved host
  timeoutMs: 30_000,        // per-request timeout
  maxRetries: 2,            // retries for 5xx / network / timeout
  userAgent: 'my-app/1.0',  // appended request identity
  fetch: globalThis.fetch,  // custom fetch (e.g. undici, a proxy agent)
});
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `apiKey` | `string` | | `vt_live_…` / `vt_test_…`. Test keys auto-route to the sandbox. |
| `bearerToken` | `string` | | Portal JWT or `vt_mcp_…` token. One of `apiKey`/`bearerToken` is required. |
| `baseUrl` | `string` | resolved from key | Explicit API origin. Wins over prefix routing. |
| `timeoutMs` | `number` | `30000` | Per-attempt timeout; aborts the request via `AbortController`. |
| `maxRetries` | `number` | `2` | Retries for `5xx`, network errors and timeouts (never for `4xx`). |
| `userAgent` | `string` | `zatabox-node/<version>` | Overrides the `User-Agent` header. |
| `fetch` | `function` | global `fetch` | Inject a custom fetch implementation. |

The client is cheap to construct and safe to reuse; create one per credential and
share it.

## How methods map to endpoints

Every endpoint is a method under a namespace: `zatabox.<namespace>.<method>(…)`. The
13 namespaces are:

`auth`, `events`, `organizer`, `eventCustomization`, `tickets`, `orders`, `payments`,
`checkin`, `community`, `growth`, `users`, `integrations`, `webhooks`.

Argument order is consistent everywhere:

```
method(pathParam1, pathParam2, …, payload?, opts?)
```

- **Path params** come first, in URL order, and are URL-encoded for you.
- **Reads** (`GET`) take an optional **query** object as `payload`.
- **Writes** (`POST/PUT/PATCH/DELETE`) take an optional **body** object as `payload`.
- **`opts`** is a trailing options bag: `{ idempotencyKey, query, headers, raw }`.

```js
await zatabox.events.list({ q: 'jazz', city: 'Lagos', limit: 20 });            // query
await zatabox.events.get('warehouse-sessions-004');                            // path param
await zatabox.organizer.updateSchedule(eventId, sessionId, { sessionTitle: 'Keynote' }); // two path params + body
await zatabox.orders.create(body, { idempotencyKey: myUuid });                 // body + opts
```

## Responses

The API wraps every success as `{ success, data, meta }`. **The SDK returns `data`
directly** you never unwrap envelopes yourself:

```js
const page = await zatabox.events.list({ limit: 20 });
// page === the `data` document, e.g. { items: [...], pagination: { cursor, has_more } }
```

`meta.request_id` and pagination cursors live inside the returned document where the
API places them (see [Pagination](#pagination)).

## Error handling

Any non-2xx response throws a **`ZataboxError`**. Network failures, timeouts and
webhook-verification failures throw one too.

```js
const { ZataboxError } = require('@zatabox/node');

try {
  await zatabox.orders.create({ items });
} catch (err) {
  if (err instanceof ZataboxError) {
    err.code;       // stable machine string, e.g. 'TICKET_SOLD_OUT'
    err.status;     // HTTP status (0 for network errors)
    err.message;    // human-readable
    err.requestId;  // 'req_01J9…' quote this to support
    err.details;    // structured context, e.g. { ticketTypeId: 'tkt_8f2k' }
  }
  throw err;
}
```

### Common error codes

| `code` | `status` | Meaning |
| --- | --- | --- |
| `VALIDATION_ERROR` | 400 | Request body failed validation (`details.issues`). |
| `UNAUTHORIZED` / `INVALID_TOKEN` | 401 | Missing/expired credential. |
| `API_KEY_REQUIRED` / `API_KEY_INVALID` | 401 | Bad or missing API key. |
| `API_KEY_INACTIVE` / `API_KEY_EXPIRED` | 403 | Key paused/expired. |
| `WRONG_ENV` | 403 | Test key on production or vice-versa. |
| `INSUFFICIENT_SCOPE` | 403 | Key lacks the scope the route needs. |
| `NOT_FOUND` | 404 | No such resource. |
| `CONFLICT` / `IDEMPOTENCY_KEY_REUSED` | 409 | Unique-constraint or idempotency clash. |
| `TICKET_SOLD_OUT` | 409 | Inventory exhausted. |
| `RATE_LIMITED` | 429 | Throttled; see `details.retryAfter` (seconds). |
| `INTERNAL_ERROR` | 500 | Server error (retried automatically). |
| `NETWORK_ERROR` | 0 | Connection/timeout failure after retries (SDK-side). |
| `MISSING_SIGNATURE` / `INVALID_SIGNATURE` / `SIGNATURE_EXPIRED` | | `webhooks.verify` failures. |

## Idempotency

Every write automatically sends an `Idempotency-Key` header (a fresh UUIDv4 per call).
The server caches the response for **24 hours**: replaying the same key + body returns
the original result; the same key with a *different* body returns
`409 IDEMPOTENCY_KEY_REUSED`. Supply your own key to make a retry safe:

```js
const key = crypto.randomUUID();
await zatabox.orders.create(cart, { idempotencyKey: key });
// …connection died, you're unsure it landed? Retry with the SAME key no double charge:
await zatabox.orders.create(cart, { idempotencyKey: key });
```

## Retries, timeouts & networking

- **Timeouts** each attempt is bounded by `timeoutMs` (default 30s) via
  `AbortController`; a timeout is treated as a retryable network error.
- **Retries** `5xx`, network errors and timeouts are retried up to `maxRetries`
  (default 2) with exponential backoff (`200ms · 2^attempt`). `4xx` is never retried.
- **Rate limits** `429` surfaces as `ZataboxError` with `code: 'RATE_LIMITED'` and
  `details.retryAfter` (seconds) when the server sends `Retry-After`.
- **Custom transport** pass `fetch` to route through a proxy/agent or to add
  instrumentation.

## Pagination

Lists are cursor-paginated. Use the built-in async iterator, which follows the cursor
across both response shapes the API uses (`pagination.cursor` or `nextCursor`):

```js
for await (const page of zatabox.paginate(zatabox.events.list, { q: 'jazz', limit: 50 })) {
  for (const ev of page.items) console.log(ev.id);
}
```

`paginate(listMethod, query)` accepts any cursor-paginated list method (e.g.
`events.list`, `users.tickets`, `webhooks.deliveries`) and your base query. To page
manually, read the cursor yourself and pass it back:

```js
let cursor;
do {
  const page = await zatabox.events.list({ limit: 50, cursor });
  // …handle page…
  cursor = page.pagination?.cursor ?? page.nextCursor ?? null;
} while (cursor);
```

## Live check-in stream (SSE)

`checkin.live` is a Server-Sent-Events stream. The SDK gives you the fully-qualified
URL; point any SSE client at it (Node has no built-in `EventSource`):

```js
const url = zatabox.checkin.liveUrl(eventId);
// Example with the `eventsource` package:
const es = new EventSource(url, { headers: { Authorization: `Bearer ${apiKey}` } });
es.addEventListener('stats', (e) => console.log(JSON.parse(e.data)));
```

## Verifying inbound webhooks

Register endpoints with `zatabox.webhooks.create({ url, events })` and verify
deliveries with `zatabox.webhooks.verify`. The signature header is
`X-Zatabox-Signature: t=<unix>,v1=<hex-hmac-sha256>`; the signed payload is
`<t>.<rawBody>`, HMAC-SHA256 with your endpoint secret, compared in constant time
(with a 5-minute timestamp tolerance).

**Always verify against the raw request body not a re-serialized object.**

```js
const express = require('express');
const app = express();

app.post('/zatabox/webhooks', express.raw({ type: '*/*' }), (req, res) => {
  try {
    const event = zatabox.webhooks.verify(
      req.body.toString('utf8'),
      req.get('X-Zatabox-Signature'),
      process.env.ZATABOX_WEBHOOK_SECRET,
    );
    switch (event.type) {
      case 'order.paid':   /* fulfil */ break;
      case 'ticket.transferred': /* … */ break;
      // browse subscribable types with zatabox.webhooks.catalog()
    }
    res.sendStatus(200);
  } catch (err) {
    res.sendStatus(400); // MISSING_SIGNATURE / INVALID_SIGNATURE / SIGNATURE_EXPIRED
  }
});
```

## End-to-end recipes

### Sell a ticket (guest checkout)

```js
const order = await zatabox.orders.create({
  items: [{ ticketTypeId, quantity: 2 }],
  guestEmail: 'fan@example.com',
  guestName: 'A. Fan',
});
const intent = await zatabox.orders.pay(order.id, { provider: 'paystack' });
// redirect the buyer to intent.checkoutUrl / handle the provider link, then:
const paid = await zatabox.payments.verify({ orderId: order.id });
```

### Check in at the gate

```js
const result = await zatabox.checkin.scan({ qrData, gateName: 'Main', deviceId });
if (result.status !== 'admitted') console.warn('denied:', result.reason);
const stats = await zatabox.checkin.stats(eventId);
```

### Request a refund (buyer)

```js
await buyer.users.createRefund({ ticketId, reason: 'Cannot attend anymore' });
```

## TypeScript

The package ships `.d.ts` declarations for the client core and every namespace, so
methods, path params, options and the `ZataboxError` type are all typed. No
`@types/*` install needed.

```ts
import { ZataboxClient, ZataboxError } from '@zatabox/node';

const z = new ZataboxClient({ apiKey: process.env.ZATABOX_API_KEY! });
const events = await z.events.list({ limit: 20 });
const order = await z.orders.create({ items: [{ ticketTypeId: 'tkt_1', quantity: 2 }] });
```

Response payloads are intentionally typed as `any` (the API returns rich, evolving
documents) decode them into your own interfaces as needed.

## Concurrency & reuse

A `ZataboxClient` is stateless apart from its credential and is safe to share across
concurrent requests. Construct one per credential at startup and reuse it; there is no
connection pool to manage beyond what your `fetch` implementation provides.

## Troubleshooting & FAQ

- **`no fetch available`** you're on Node ≤16; upgrade to Node 18+ or pass
  `{ fetch }` (e.g. from `undici`).
- **`403 WRONG_ENV`** you used a `vt_test_` key against production or a `vt_live_`
  key against the sandbox. Match the key to the environment.
- **`401 API_KEY_REQUIRED`** the key didn't start with `vt_`, or no credential was
  provided. Mint a key in the portal/sandbox console.
- **`409 IDEMPOTENCY_KEY_REUSED`** you reused an `idempotencyKey` with a different
  body. Use a fresh key for a genuinely new request.
- **Webhook always `INVALID_SIGNATURE`** you verified a parsed/re-serialized body.
  Capture the **raw** bytes (`express.raw`) and pass that exact string.
- **A list looks truncated** it's paginated; iterate with `zatabox.paginate(…)`.

## Versioning & support

- SemVer; the current version is exported as `VERSION`.
- API base: `https://api.zatabox.com/api/v1` · Docs: <https://zatabox.com/docs> ·
  Support: developers@zatabox.com.

## License

MIT

---

## Full endpoint reference

<!-- BEGIN ENDPOINTS (generated by scripts/generate.mjs do not edit) -->

The SDK exposes **78 endpoints** across **13 namespaces**. Every method is listed below with its idiomatic signature, the underlying HTTP route, and what it does. Path parameters are positional; reads take an optional query map and writes take an optional body, both followed by a call-options bag.

### `client.auth` Auth

Account registration, password + passwordless sign-in, 2FA login and token refresh.

| Method | Endpoint | Description |
| --- | --- | --- |
| `register(body?, opts?)` | `POST /api/v1/auth/register` | Register an account; returns the user plus an accessToken/refreshToken pair. |
| `login(body?, opts?)` | `POST /api/v1/auth/login` | Log in with email + password; returns a JWT pair (or a 2FA challenge). |
| `loginVerify2fa(body?, opts?)` | `POST /api/v1/auth/2fa-verify` | Complete a 2FA login challenge; returns the JWT pair. |
| `requestToken(body?, opts?)` | `POST /api/v1/auth/token/request` | Passwordless: email a buyer a 6-digit login code. |
| `exchangeToken(body?, opts?)` | `POST /api/v1/auth/token/exchange` | Passwordless: exchange email + 6-digit code for a JWT pair. |
| `refresh(body?, opts?)` | `POST /api/v1/auth/refresh` | Refresh an expired access token (rotates the refresh token). |
| `logout(body?, opts?)` | `POST /api/v1/auth/logout` | Revoke a refresh token. |

### `client.events` Events (Public)

Public event discovery and read, plus external ticket issuance.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query?, opts?)` | `GET /api/v1/events` | List and search published public events (cursor-paginated). |
| `get(slug, query?, opts?)` | `GET /api/v1/events/:slug` | Event detail by slug (organizer info, schedule, active ticket types). |
| `tickets(id, query?, opts?)` | `GET /api/v1/events/:id/tickets` | List an event's ticket types with live availability. |
| `issue(eventId, body?, opts?)` | `POST /api/v1/events/:eventId/issue` | Issue tickets you sold elsewhere (developer-handled payment; 3% wallet fee on paid tickets). |

### `client.organizer` Organizer

Organizer surface: organization read, events, ticket types, schedule sessions, seating sections and promo codes.

| Method | Endpoint | Description |
| --- | --- | --- |
| `getOrganization(id, query?, opts?)` | `GET /api/v1/organizer/organizations/:id` | Get organization details and per-currency wallet balances. |
| `createEvent(body?, opts?)` | `POST /api/v1/organizer/events` | Create a draft event. |
| `updateEvent(id, body?, opts?)` | `PUT /api/v1/organizer/events/:id` | Partial-update an event. |
| `publishEvent(id, body?, opts?)` | `POST /api/v1/organizer/events/:id/publish` | Publish a draft event. |
| `unpublishEvent(id, body?, opts?)` | `POST /api/v1/organizer/events/:id/unpublish` | Unpublish a published event back to draft. |
| `deleteEvent(id, body?, opts?)` | `DELETE /api/v1/organizer/events/:id` | Cancel an event. |
| `createTicket(id, body?, opts?)` | `POST /api/v1/organizer/events/:id/tickets` | Create a ticket type. |
| `schedule(id, query?, opts?)` | `GET /api/v1/organizer/events/:id/schedule` | List schedule sessions (running order). |
| `createSchedule(id, body?, opts?)` | `POST /api/v1/organizer/events/:id/schedule` | Add a schedule session. |
| `updateSchedule(id, sessionId, body?, opts?)` | `PUT /api/v1/organizer/events/:id/schedule/:sessionId` | Update a schedule session. |
| `deleteSchedule(id, sessionId, body?, opts?)` | `DELETE /api/v1/organizer/events/:id/schedule/:sessionId` | Delete a schedule session. |
| `sections(id, query?, opts?)` | `GET /api/v1/organizer/events/:id/sections` | List seating/capacity sections. |
| `createSection(id, body?, opts?)` | `POST /api/v1/organizer/events/:id/sections` | Add a seating section. |
| `updateSection(id, sectionId, body?, opts?)` | `PUT /api/v1/organizer/events/:id/sections/:sectionId` | Update a seating section. |
| `deleteSection(id, sectionId, body?, opts?)` | `DELETE /api/v1/organizer/events/:id/sections/:sectionId` | Delete a seating section. |
| `promoCodes(query?, opts?)` | `GET /api/v1/organizer/promo-codes` | List promo codes (optionally filtered by event). |
| `createPromoCode(body?, opts?)` | `POST /api/v1/organizer/promo-codes` | Create a promo code. |
| `updatePromoCode(id, body?, opts?)` | `PUT /api/v1/organizer/promo-codes/:id` | Update a promo code. |
| `deletePromoCode(id, body?, opts?)` | `DELETE /api/v1/organizer/promo-codes/:id` | Delete or disable a promo code. |

### `client.eventCustomization` Event page customization

Per-event public-page theming and the “Good to know” FAQ.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get(id, query?, opts?)` | `GET /api/v1/organizer/event-customization/:id` | Get an event's page customization (theme, layout, FAQ, SEO). |
| `update(id, body?, opts?)` | `PUT /api/v1/organizer/event-customization/:id` | Update an event's page customization (incl. the FAQ list). |

### `client.tickets` Tickets

Checkout-time ticket helpers.

| Method | Endpoint | Description |
| --- | --- | --- |
| `validatePromo(body?, opts?)` | `POST /api/v1/tickets/promo/validate` | Validate a promo code against a cart (read-only preview, does not consume a use). |

### `client.orders` Orders

Carted checkout: create, read, pay, cancel.

| Method | Endpoint | Description |
| --- | --- | --- |
| `create(body?, opts?)` | `POST /api/v1/orders` | Create an order (guest checkout needs only name + email). |
| `get(id, query?, opts?)` | `GET /api/v1/orders/:id` | Get an order (pass ?token for guest reads). |
| `pay(id, body?, opts?)` | `POST /api/v1/orders/:id/pay` | Initiate payment (provider: nowpayments \| paystack \| flutterwave). |
| `cancel(id, body?, opts?)` | `POST /api/v1/orders/:id/cancel` | Cancel an unpaid order and release held inventory. |

### `client.payments` Payments

Verify charges, read payment status, list crypto coins.

| Method | Endpoint | Description |
| --- | --- | --- |
| `verify(body?, opts?)` | `POST /api/v1/payments/verify` | Actively verify a payment with the provider and issue tickets (idempotent, poll-safe). |
| `get(orderId, query?, opts?)` | `GET /api/v1/payments/:orderId` | Read payment/order status and attempts (read-only). |
| `cryptoCurrencies(query?, opts?)` | `GET /api/v1/payments/crypto/currencies` | List supported NOWPayments crypto coins (for the payCurrency value). |

### `client.checkin` Check-in

Gate scanning, offline manifests + sync, live stats.

| Method | Endpoint | Description |
| --- | --- | --- |
| `scan(body?, opts?)` | `POST /api/v1/checkin/scan` | Validate a QR, barcode or 6-character door code at the gate. |
| `manual(id, body?, opts?)` | `POST /api/v1/checkin/event/:id/manual` | Manually check in a typed ticket code. |
| `manifest(id, query?, opts?)` | `GET /api/v1/checkin/event/:id/manifest` | Hashed guest-list manifest for offline scanning (pass ?since for a delta). |
| `batch(body?, opts?)` | `POST /api/v1/checkin/batch` | Sync up to 500 queued offline scans. |
| `stats(id, query?, opts?)` | `GET /api/v1/checkin/event/:id/stats` | Check-in totals, capacity %, entry rate and per-gate breakdown. |
| `gate(id, gate, query?, opts?)` | `GET /api/v1/checkin/event/:id/gate/:gate` | Per-gate check-in stats slice. |
| `liveUrl(id, query?)` | `GET /api/v1/checkin/event/:id/live` | Server-Sent Events stream a stats snapshot every 2 seconds. |

### `client.community` Community

Verified-attendee reviews, organizer follows/subscribers and event waitlists.

| Method | Endpoint | Description |
| --- | --- | --- |
| `submitReview(body?, opts?)` | `POST /api/v1/community/reviews` | Review an event (checked-in ticket holders only; ticketCode + email prove attendance). |
| `follow(orgId, body?, opts?)` | `POST /api/v1/community/orgs/:orgId/follow` | Follow an organizer (subscribe to new-event announcements). |
| `followers(orgId, query?, opts?)` | `GET /api/v1/community/orgs/:orgId/followers` | List an organizer's subscribers (organizer auth). |
| `removeFollower(orgId, followerId, body?, opts?)` | `DELETE /api/v1/community/orgs/:orgId/followers/:followerId` | Remove a subscriber (organizer auth). |
| `joinWaitlist(eventId, body?, opts?)` | `POST /api/v1/community/events/:eventId/waitlist` | Join an event waitlist (offers fire on cancellations). |

### `client.growth` Growth (Organizer)

Comp tickets, CSV import, broadcasts and attendee tags.

| Method | Endpoint | Description |
| --- | --- | --- |
| `mintComps(eventId, body?, opts?)` | `POST /api/v1/organizer/growth/events/:eventId/comps` | Bulk-mint and email complimentary tickets. |
| `importCompsCsv(eventId, body?, opts?)` | `POST /api/v1/organizer/growth/events/:eventId/comps/import-csv` | Import attendees (comp tickets) from CSV. |
| `broadcastEvent(eventId, body?, opts?)` | `POST /api/v1/organizer/growth/events/:eventId/broadcast` | Email a broadcast to an event's attendees (replies thread to the organizer inbox). |
| `addTags(body?, opts?)` | `POST /api/v1/organizer/growth/tags` | Tag attendees (additive; powers broadcast filters and CRM segments). |
| `removeTag(body?, opts?)` | `DELETE /api/v1/organizer/growth/tags` | Remove an attendee tag. |

### `client.users` Buyers

The authenticated buyer: profile, ticket wallet, data export, refunds, reports and organizer messaging.

| Method | Endpoint | Description |
| --- | --- | --- |
| `me(query?, opts?)` | `GET /api/v1/users/me` | Current buyer profile. |
| `tickets(query?, opts?)` | `GET /api/v1/users/me/tickets` | The buyer's ticket wallet (cursor-paginated). |
| `export(query?, opts?)` | `GET /api/v1/users/me/export` | GDPR data export one JSON download of everything on the account. |
| `createRefund(body?, opts?)` | `POST /api/v1/users/me/refunds` | Request a refund for a ticket. |
| `createReport(body?, opts?)` | `POST /api/v1/users/me/reports` | File a report against an event or organizer. |
| `messages(query?, opts?)` | `GET /api/v1/users/me/messages` | The buyer's message threads with organizers. |
| `sendTicketMessage(ticketId, body?, opts?)` | `POST /api/v1/users/me/tickets/:ticketId/message` | Message the organizer about a ticket (rate-limited). |

### `client.integrations` API keys

Manage your organization's own API keys (organizer owner/admin auth).

| Method | Endpoint | Description |
| --- | --- | --- |
| `createApiKey(orgId, body?, opts?)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys` | Create an API key (plaintext secret returned exactly once). |
| `listApiKeys(orgId, query?, opts?)` | `GET /api/v1/organizer/integrations/org/:orgId/api-keys` | List API keys (prefixes and metadata only, never the secret). |
| `updateApiKey(orgId, keyId, body?, opts?)` | `PUT /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Update a key (rename, pause, re-scope). |
| `rotateApiKey(orgId, keyId, body?, opts?)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId/rotate` | Rotate a key's secret (new secret returned once; old one invalidated). |
| `deleteApiKey(orgId, keyId, body?, opts?)` | `DELETE /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Revoke an API key. |

### `client.webhooks` Webhooks

Register webhook endpoints, manage secrets, inspect and replay deliveries. (Use webhooks.verify() to validate inbound signatures.)

| Method | Endpoint | Description |
| --- | --- | --- |
| `create(body?, opts?)` | `POST /api/v1/webhooks` | Create a webhook endpoint (signing secret returned exactly once). |
| `list(query?, opts?)` | `GET /api/v1/webhooks` | List webhook endpoints. |
| `update(id, body?, opts?)` | `PUT /api/v1/webhooks/:id` | Update a webhook endpoint. |
| `delete(id, body?, opts?)` | `DELETE /api/v1/webhooks/:id` | Delete a webhook endpoint. |
| `test(id, body?, opts?)` | `POST /api/v1/webhooks/:id/test` | Send a signed test event to the endpoint. |
| `rotateSecret(id, body?, opts?)` | `POST /api/v1/webhooks/:id/rotate-secret` | Rotate the signing secret (new secret returned once). |
| `deliveries(id, query?, opts?)` | `GET /api/v1/webhooks/:id/deliveries` | List delivery attempts for an endpoint. |
| `replay(id, body?, opts?)` | `POST /api/v1/webhooks/deliveries/:id/replay` | Replay a past delivery. |
| `catalog(query?, opts?)` | `GET /api/v1/webhooks/catalog` | List every subscribable event type (no auth). |

<!-- END ENDPOINTS -->
