# @zatabox/node

Official **Node.js SDK** for the [Zatabox Tickets](https://zatabox.com) REST API the
white-label event-ticketing platform where the box office is an API. This SDK is a
thin, fully-typed client over `https://api.zatabox.com/api/v1` that handles auth,
sandbox routing, idempotency, retries, pagination, binary downloads, file uploads and
webhook verification for you.

- **Zero runtime dependencies** built on the global `fetch` and `node:crypto`.
- **Complete** every one of the **244 REST endpoints** is a typed method.
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
- [Binary downloads (PDF / CSV)](#binary-downloads-pdf--csv)
- [Live check-in stream (SSE)](#live-check-in-stream-sse)
- [File uploads](#file-uploads)
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

This SDK is **distributed via GitHub** — it is not published to npm. It lives in the
`node/` directory of
[`mysmartrobot/zataboxtickets`](https://github.com/mysmartrobot/zataboxtickets).
Because npm cannot install a subdirectory of a git repo directly, use one of:

**Option A — clone, then install the local path** (recommended):

```bash
git clone https://github.com/mysmartrobot/zataboxtickets.git
npm install ./zataboxtickets/node
```

This records `"@zatabox/node": "file:./zataboxtickets/node"` in your `package.json`.
Update later with `git -C zataboxtickets pull && npm install ./zataboxtickets/node`.

**Option B — git submodule** (pinned and easy to update):

```bash
git submodule add https://github.com/mysmartrobot/zataboxtickets.git vendor/zataboxtickets
npm install ./vendor/zataboxtickets/node
```

**Option C — vendor the folder**: copy the repo's `node/` directory into your project
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
with your production account). A test key used against production or vice-versa —
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
26 namespaces are:

`auth`, `users`, `savedSearches`, `dataExport`, `events`, `tickets`, `orders`,
`payments`, `checkin`, `scan`, `search`, `media`, `organizer`, `wallets`,
`scannerTokens`, `integrations`, `eventCustomization`, `growth`, `publicEvents`,
`site`, `community`, `track`, `webhooks`, `whiteLabel`, `support`, `util`.

Argument order is consistent everywhere:

```
method(pathParam1, pathParam2, …, payload?, opts?)
```

- **Path params** come first, in URL order, and are URL-encoded for you.
- **Reads** (`GET`) take an optional **query** object as `payload`.
- **Writes** (`POST/PUT/PATCH/DELETE`) take an optional **body** object as `payload`.
- **`opts`** is a trailing options bag: `{ idempotencyKey, query, headers, raw }`.

```js
await zatabox.events.list({ q: 'jazz', city: 'Lagos', limit: 20 });        // query
await zatabox.events.get('warehouse-sessions-004');                        // path param
await zatabox.organizer.updateTicket(eventId, ticketId, { price: 7500 });  // two path params + body
await zatabox.orders.create(body, { idempotencyKey: myUuid });             // body + opts
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
for await (const page of zatabox.paginate(zatabox.organizer.events, { limit: 50 })) {
  for (const ev of page.events ?? page.items) console.log(ev.id);
}
```

`paginate(listMethod, query)` accepts any list method and your base query. To page
manually, read the cursor yourself and pass it back:

```js
let cursor;
do {
  const page = await zatabox.organizer.events({ limit: 50, cursor });
  // …handle page…
  cursor = page.pagination?.cursor ?? page.nextCursor ?? null;
} while (cursor);
```

## Binary downloads (PDF / CSV)

`tickets.pdf`, `orders.invoice`, `checkin.export`, `organizer.eventExport` and
`site.sitemap` return raw bytes instead of JSON:

```js
const { data, contentType, filename } = await zatabox.tickets.pdf(ticketId);
// data is a Buffer
require('fs').writeFileSync(filename ?? 'ticket.pdf', data);
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

## File uploads

`media.upload` sends `multipart/form-data` (built with the platform `FormData`/`Blob`):

```js
const buf = require('fs').readFileSync('cover.jpg');
const asset = await zatabox.media.upload(buf, {
  filename: 'cover.jpg',
  contentType: 'image/jpeg',
  // field: 'file',          // form field name (default "file")
  // fields: { alt: 'Cover' } // extra form fields
});
// Use asset.id / asset.url as an event coverImage, etc.
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

### Issue and inspect a refund (buyer)

```js
await buyer.users.createRefund({ ticketId, reason: 'Cannot attend anymore' });
const refunds = await buyer.users.refunds();
```

## TypeScript

The package ships `.d.ts` declarations for the client core and every namespace, so
methods, path params, options and the `ZataboxError`/`BinaryResponse` types are all
typed. No `@types/*` install needed.

```ts
import { ZataboxClient, ZataboxError, BinaryResponse } from '@zatabox/node';

const z = new ZataboxClient({ apiKey: process.env.ZATABOX_API_KEY! });
const events = await z.events.list({ limit: 20 });
const pdf: BinaryResponse = await z.tickets.pdf('5');
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

The SDK exposes **244 endpoints** across **26 namespaces**. Every method is listed below with its idiomatic signature, the underlying HTTP route, and what it does. Path parameters are positional; reads take an optional query map and writes take an optional body, both followed by a call-options bag.

### `client.auth` Auth

Registration, password + passwordless sign-in, token refresh, 2FA and verification.

| Method | Endpoint | Description |
| --- | --- | --- |
| `register(body?, opts?)` | `POST /api/v1/auth/register` | Register a new user with email + password. |
| `login(body?, opts?)` | `POST /api/v1/auth/login` | Sign in with email + password; returns the JWT pair (or a 2FA challenge). |
| `loginVerify2fa(body?, opts?)` | `POST /api/v1/auth/2fa-verify` | Complete a login that returned a 2FA challenge. |
| `refresh(body?, opts?)` | `POST /api/v1/auth/refresh` | Exchange a refresh token for a fresh access/refresh pair. |
| `logout(body?, opts?)` | `POST /api/v1/auth/logout` | Revoke a refresh token. |
| `forgotPassword(body?, opts?)` | `POST /api/v1/auth/forgot-password` | Email a password-reset link. |
| `resetPassword(body?, opts?)` | `POST /api/v1/auth/reset-password` | Set a new password using a reset token. |
| `requestToken(body?, opts?)` | `POST /api/v1/auth/token/request` | Passwordless: email a 6-digit login code. |
| `exchangeToken(body?, opts?)` | `POST /api/v1/auth/token/exchange` | Passwordless: swap an emailed code for the JWT pair. |
| `verifyOtp(body?, opts?)` | `POST /api/v1/auth/verify-otp` | Verify a one-time passcode. |
| `verifyEmail(body?, opts?)` | `POST /api/v1/auth/verify-email` | Confirm an email address from a verification token. |
| `verifyPhone(body?, opts?)` | `POST /api/v1/auth/verify-phone` | Confirm a phone number from an SMS code. |
| `loginOauth(body?, opts?)` | `POST /api/v1/auth/login/oauth` | Sign in with a third-party OAuth identity token. |
| `enable2fa(body?, opts?)` | `POST /api/v1/auth/2fa/enable` | Begin enrolling TOTP two-factor auth. |
| `verify2fa(body?, opts?)` | `POST /api/v1/auth/2fa/verify` | Confirm a TOTP code to finish 2FA enrollment. |

### `client.users` Users (Buyer)

The authenticated account: profile, wallet, tickets, orders, refunds, reports, messaging and notifications.

| Method | Endpoint | Description |
| --- | --- | --- |
| `me(query?, opts?)` | `GET /api/v1/users/me` | Current user profile. |
| `updateMe(body?, opts?)` | `PUT /api/v1/users/me` | Update the current user profile. |
| `orders(query?, opts?)` | `GET /api/v1/users/me/orders` | List the buyer's orders. |
| `tickets(query?, opts?)` | `GET /api/v1/users/me/tickets` | List the buyer's tickets across all organizers. |
| `deleteAccount(body?, opts?)` | `DELETE /api/v1/users/me` | Close the account. |
| `changePassword(body?, opts?)` | `POST /api/v1/users/me/password` | Change the account password. |
| `activity(query?, opts?)` | `GET /api/v1/users/me/activity` | Recent account activity. |
| `loginInfo(query?, opts?)` | `GET /api/v1/users/me/login-info` | Last-login metadata. |
| `twofaStatus(query?, opts?)` | `GET /api/v1/users/me/2fa/status` | Whether 2FA is enabled. |
| `twofaSetup(body?, opts?)` | `POST /api/v1/users/me/2fa/setup` | Start 2FA setup (returns the TOTP secret/QR). |
| `twofaEnable(body?, opts?)` | `POST /api/v1/users/me/2fa/enable` | Enable 2FA after verifying a code. |
| `twofaDisable(body?, opts?)` | `POST /api/v1/users/me/2fa/disable` | Disable 2FA. |
| `createRefund(body?, opts?)` | `POST /api/v1/users/me/refunds` | Submit a refund request for a ticket. |
| `refunds(query?, opts?)` | `GET /api/v1/users/me/refunds` | List the buyer's refund requests. |
| `withdrawRefund(id, body?, opts?)` | `POST /api/v1/users/me/refunds/:id/withdraw` | Withdraw a pending refund request. |
| `createReport(body?, opts?)` | `POST /api/v1/users/me/reports` | File a report against an event or organizer. |
| `reports(query?, opts?)` | `GET /api/v1/users/me/reports` | List the buyer's filed reports. |
| `messages(query?, opts?)` | `GET /api/v1/users/me/messages` | Message threads with organizers. |
| `messageThread(threadId, query?, opts?)` | `GET /api/v1/users/me/messages/:threadId` | A single message thread. |
| `sendTicketMessage(ticketId, body?, opts?)` | `POST /api/v1/users/me/tickets/:ticketId/message` | Message the organizer of a ticket. |
| `notifications(query?, opts?)` | `GET /api/v1/users/me/notifications` | In-app notifications. |
| `notificationsUnreadCount(query?, opts?)` | `GET /api/v1/users/me/notifications/unread-count` | Count of unread notifications. |
| `markNotificationsRead(body?, opts?)` | `POST /api/v1/users/me/notifications/read` | Mark notifications as read. |
| `updateAvatar(body?, opts?)` | `PUT /api/v1/users/me/avatar` | Update the account avatar. |
| `updateNotificationSettings(body?, opts?)` | `PUT /api/v1/users/me/notifications/settings` | Update notification preferences. |

### `client.savedSearches` Saved searches

The buyer's saved discovery searches.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query?, opts?)` | `GET /api/v1/users/me/saved-searches` | List saved searches. |
| `create(body?, opts?)` | `POST /api/v1/users/me/saved-searches` | Save a search. |
| `delete(id, body?, opts?)` | `DELETE /api/v1/users/me/saved-searches/:id` | Delete a saved search. |

### `client.dataExport` Data export

GDPR-style export of the account's data.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get(query?, opts?)` | `GET /api/v1/users/me/export` | Export all of the account's data. |

### `client.events` Events (Public)

Public event discovery rails and per-event read panels.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query?, opts?)` | `GET /api/v1/events` | List / search public events (cursor-paginated). |
| `trending(query?, opts?)` | `GET /api/v1/events/trending` | Trending events. |
| `categories(query?, opts?)` | `GET /api/v1/events/categories` | Event categories with counts. |
| `nearby(query?, opts?)` | `GET /api/v1/events/nearby` | Events near a lat/lng or city. |
| `newThisWeek(query?, opts?)` | `GET /api/v1/events/new-this-week` | Recently published events. |
| `endingSoon(query?, opts?)` | `GET /api/v1/events/ending-soon` | Events with sales ending soon. |
| `free(query?, opts?)` | `GET /api/v1/events/free` | Free events. |
| `recommended(query?, opts?)` | `GET /api/v1/events/recommended` | Personalized recommendations. |
| `schedule(id, query?, opts?)` | `GET /api/v1/events/:id/schedule` | An event's session schedule. |
| `organizer(id, query?, opts?)` | `GET /api/v1/events/:id/organizer` | Public organizer profile for an event. |
| `faq(id, query?, opts?)` | `GET /api/v1/events/:id/faq` | An event's FAQ. |
| `related(id, query?, opts?)` | `GET /api/v1/events/:id/related` | Related events. |
| `expressInterest(id, body?, opts?)` | `POST /api/v1/events/:id/interest` | Register interest in an event. |
| `issue(eventId, body?, opts?)` | `POST /api/v1/events/:eventId/issue` | Externally issue a ticket for an event (integrator mode). |
| `get(slug, query?, opts?)` | `GET /api/v1/events/:slug` | Get a public event by slug. |
| `tickets(id, query?, opts?)` | `GET /api/v1/events/:id/tickets` | List an event's purchasable ticket types. |

### `client.tickets` Tickets

Ticket QR/PDF, peer-to-peer transfers, promo validation and wallet passes.

| Method | Endpoint | Description |
| --- | --- | --- |
| `validatePromo(body?, opts?)` | `POST /api/v1/tickets/promo/validate` | Validate a promo code against a cart. |
| `qr(id, query?, opts?)` | `GET /api/v1/tickets/:id/qr` | Current rotating QR payload for a ticket (JSON). |
| `pdf(id, query?, opts?) → BinaryResponse` | `GET /api/v1/tickets/:id/pdf` | Ticket PDF (application/pdf bytes). |
| `transfer(id, body?, opts?)` | `POST /api/v1/tickets/:id/transfer` | Initiate a peer-to-peer transfer; the recipient gets a claim link. |
| `revokeTransfer(transferId, body?, opts?)` | `POST /api/v1/tickets/transfers/:transferId/revoke` | Revoke a still-pending transfer (initiator only). |
| `getTransfer(token, query?, opts?)` | `GET /api/v1/tickets/transfers/claim/:token` | Inspect a pending transfer by claim token. |
| `claimTransfer(token, body?, opts?)` | `POST /api/v1/tickets/transfers/claim/:token` | Claim a transfer; rewrites the ticket holder to the recipient. |
| `walletPass(id, query?, opts?)` | `GET /api/v1/tickets/:id/wallet-pass` | Google/Apple wallet 'Save to Wallet' link (JSON). |
| `listByEvent(eventId, query?, opts?)` | `GET /api/v1/tickets/:eventId` | List ticket types for an event (legacy /ticket-types mount). |

### `client.orders` Orders

Carted checkout: create, pay, cancel, invoice.

| Method | Endpoint | Description |
| --- | --- | --- |
| `create(body?, opts?)` | `POST /api/v1/orders` | Create an order (reserves inventory). |
| `get(id, query?, opts?)` | `GET /api/v1/orders/:id` | Order detail. |
| `cancel(id, body?, opts?)` | `POST /api/v1/orders/:id/cancel` | Cancel an order and release its hold. |
| `pay(id, body?, opts?)` | `POST /api/v1/orders/:id/pay` | Initiate payment for an order (nowpayments \| paystack \| flutterwave). |
| `invoice(id, query?, opts?) → BinaryResponse` | `GET /api/v1/orders/:id/invoice` | Order receipt PDF (application/pdf bytes). |

### `client.payments` Payments

Payment intent creation, verification, method discovery, and inbound provider webhooks.

| Method | Endpoint | Description |
| --- | --- | --- |
| `verify(body?, opts?)` | `POST /api/v1/payments/verify` | Confirm a charge with the provider and complete the order (idempotent). |
| `get(orderId, query?, opts?)` | `GET /api/v1/payments/:orderId` | Payment status for an order. |
| `initiate(body?, opts?)` | `POST /api/v1/payments/initiate` | Create a payment intent. |
| `methods(query?, opts?)` | `GET /api/v1/payments/methods` | Available payment methods for a currency. |
| `cryptoCurrencies(query?, opts?)` | `GET /api/v1/payments/crypto/currencies` | Supported crypto currencies (NOWPayments). |
| `webhookNowpayments(body?, opts?)` | `POST /api/v1/payments/webhook/nowpayments` | Inbound NOWPayments webhook (provider callback; not for client use). |
| `webhookPaystack(body?, opts?)` | `POST /api/v1/payments/webhook/paystack` | Inbound Paystack webhook (provider callback; not for client use). |
| `webhookFlutterwave(body?, opts?)` | `POST /api/v1/payments/webhook/flutterwave` | Inbound Flutterwave webhook (provider callback; not for client use). |

### `client.checkin` Check-in

Gate scanning, offline manifests, batch sync, live stats and CSV export.

| Method | Endpoint | Description |
| --- | --- | --- |
| `scan(body?, opts?)` | `POST /api/v1/checkin/scan` | Validate a QR / short-code ticket at a gate. |
| `batch(body?, opts?)` | `POST /api/v1/checkin/batch` | Flush a queue of scans captured offline. |
| `manual(id, body?, opts?)` | `POST /api/v1/checkin/event/:id/manual` | Manually check in an attendee. |
| `manifest(id, query?, opts?)` | `GET /api/v1/checkin/event/:id/manifest` | Offline manifest (ticket hashes + statuses); pass ?since for a delta. |
| `stats(id, query?, opts?)` | `GET /api/v1/checkin/event/:id/stats` | Live check-in stats snapshot. |
| `registerDevice(body?, opts?)` | `POST /api/v1/checkin/device/register` | Register a scanning device. |
| `liveUrl(id, query?)` | `GET /api/v1/checkin/event/:id/live` | Server-Sent Events stream of live check-in stats. |
| `gate(id, gate, query?, opts?)` | `GET /api/v1/checkin/event/:id/gate/:gate` | Per-gate check-in stats. |
| `export(id, query?, opts?) → BinaryResponse` | `GET /api/v1/checkin/event/:id/export` | Check-in log CSV (text/csv bytes). |

### `client.scan` Scanner-token check-in

Passwordless gate scanning using a short-lived scanner token (the /scan kiosk surface).

| Method | Endpoint | Description |
| --- | --- | --- |
| `exchange(body?, opts?)` | `POST /api/v1/checkin-token/exchange` | Exchange a scanner token for a scoped session. |
| `session(query?, opts?)` | `GET /api/v1/checkin-token/me` | Current scanner session (event + gate context). |
| `scan(body?, opts?)` | `POST /api/v1/checkin-token/scan` | Validate a ticket with the scanner session. |
| `manifest(query?, opts?)` | `GET /api/v1/checkin-token/manifest` | Offline manifest for the scanner session. |
| `batch(body?, opts?)` | `POST /api/v1/checkin-token/batch` | Flush offline scans for the scanner session. |

### `client.search` Search

Full-text + faceted event search.

| Method | Endpoint | Description |
| --- | --- | --- |
| `query(query?, opts?)` | `GET /api/v1/search` | Full-text + faceted search. |
| `suggest(query?, opts?)` | `GET /api/v1/search/suggest` | Type-ahead suggestions. |
| `trending(query?, opts?)` | `GET /api/v1/search/trending` | Trending search terms. |
| `popular(city, query?, opts?)` | `GET /api/v1/search/popular/:city` | Popular searches in a city. |

### `client.media` Media

Image/asset uploads and the stable /media/:id resolver.

| Method | Endpoint | Description |
| --- | --- | --- |
| `upload(body?, opts?)` | `POST /api/v1/media/upload` | Upload an image/asset (multipart/form-data). |

### `client.organizer` Organizer

Authenticated organizer surface: organizations, members, events, ticket types, schedules, sections, promo codes, attendees, payouts, refunds, reports and messaging.

| Method | Endpoint | Description |
| --- | --- | --- |
| `setup(body?, opts?)` | `POST /api/v1/organizer/setup` | Bootstrap an organizer account + first organization. |
| `me(query?, opts?)` | `GET /api/v1/organizer/me` | Current organizer context (orgs + memberships). |
| `createOrganization(body?, opts?)` | `POST /api/v1/organizer/organizations` | Create an organization. |
| `getOrganization(orgId, query?, opts?)` | `GET /api/v1/organizer/organizations/:orgId` | Organization detail. |
| `updateOrganization(orgId, body?, opts?)` | `PUT /api/v1/organizer/organizations/:orgId` | Update an organization. |
| `setOrganizationStatus(orgId, body?, opts?)` | `PUT /api/v1/organizer/organizations/:orgId/status` | Change an organization's status. |
| `members(orgId, query?, opts?)` | `GET /api/v1/organizer/organizations/:orgId/members` | List organization members. |
| `invite(orgId, body?, opts?)` | `POST /api/v1/organizer/organizations/:orgId/invites` | Invite a member. |
| `resendInvite(orgId, memberId, body?, opts?)` | `POST /api/v1/organizer/organizations/:orgId/invites/:memberId/resend` | Resend a member invite. |
| `removeMember(orgId, memberId, body?, opts?)` | `DELETE /api/v1/organizer/organizations/:orgId/members/:memberId` | Remove a member. |
| `events(query?, opts?)` | `GET /api/v1/organizer/events` | List the organizer's events. |
| `createEvent(body?, opts?)` | `POST /api/v1/organizer/events` | Create a draft event. |
| `getEvent(id, query?, opts?)` | `GET /api/v1/organizer/events/:id` | Organizer event detail. |
| `updateEvent(id, body?, opts?)` | `PUT /api/v1/organizer/events/:id` | Update an event. |
| `setEventStatus(id, body?, opts?)` | `PUT /api/v1/organizer/events/:id/status` | Change an event's status. |
| `publishEvent(id, body?, opts?)` | `POST /api/v1/organizer/events/:id/publish` | Publish a draft event. |
| `unpublishEvent(id, body?, opts?)` | `POST /api/v1/organizer/events/:id/unpublish` | Unpublish a published event back to draft. |
| `deleteEvent(id, body?, opts?)` | `DELETE /api/v1/organizer/events/:id` | Cancel/delete an event. |
| `tickets(id, query?, opts?)` | `GET /api/v1/organizer/events/:id/tickets` | List an event's ticket types. |
| `createTicket(id, body?, opts?)` | `POST /api/v1/organizer/events/:id/tickets` | Create a ticket type. |
| `updateTicket(id, tid, body?, opts?)` | `PUT /api/v1/organizer/events/:id/tickets/:tid` | Update a ticket type. |
| `deleteTicket(id, tid, body?, opts?)` | `DELETE /api/v1/organizer/events/:id/tickets/:tid` | Delete a ticket type. |
| `schedule(id, query?, opts?)` | `GET /api/v1/organizer/events/:id/schedule` | List schedule sessions. |
| `createSchedule(id, body?, opts?)` | `POST /api/v1/organizer/events/:id/schedule` | Add a schedule session. |
| `updateSchedule(id, sid, body?, opts?)` | `PUT /api/v1/organizer/events/:id/schedule/:sid` | Update a schedule session. |
| `deleteSchedule(id, sid, body?, opts?)` | `DELETE /api/v1/organizer/events/:id/schedule/:sid` | Delete a schedule session. |
| `sections(id, query?, opts?)` | `GET /api/v1/organizer/events/:id/sections` | List seating/venue sections. |
| `createSection(id, body?, opts?)` | `POST /api/v1/organizer/events/:id/sections` | Add a section. |
| `updateSection(id, sid, body?, opts?)` | `PUT /api/v1/organizer/events/:id/sections/:sid` | Update a section. |
| `deleteSection(id, sid, body?, opts?)` | `DELETE /api/v1/organizer/events/:id/sections/:sid` | Delete a section. |
| `promoCodes(query?, opts?)` | `GET /api/v1/organizer/promo-codes` | List promo codes. |
| `createPromoCode(body?, opts?)` | `POST /api/v1/organizer/promo-codes` | Create a promo code. |
| `updatePromoCode(id, body?, opts?)` | `PUT /api/v1/organizer/promo-codes/:id` | Update a promo code. |
| `deletePromoCode(id, body?, opts?)` | `DELETE /api/v1/organizer/promo-codes/:id` | Delete a promo code. |
| `eventAnalytics(id, query?, opts?)` | `GET /api/v1/organizer/events/:id/analytics` | Sales/analytics for an event. |
| `eventAttendees(id, query?, opts?)` | `GET /api/v1/organizer/events/:id/attendees` | Attendee CRM list for an event. |
| `eventExport(id, query?, opts?) → BinaryResponse` | `GET /api/v1/organizer/events/:id/export` | Attendee export CSV (text/csv bytes). |
| `payouts(query?, opts?)` | `GET /api/v1/organizer/payouts` | List payouts. |
| `payout(id, query?, opts?)` | `GET /api/v1/organizer/payouts/:id` | Payout detail. |
| `requestPayout(body?, opts?)` | `POST /api/v1/organizer/payouts/request` | Request a payout. |
| `updatePayoutSettings(body?, opts?)` | `PUT /api/v1/organizer/payout-settings` | Update payout settings. |
| `refunds(orgId, query?, opts?)` | `GET /api/v1/organizer/orgs/:orgId/refunds` | List refund requests for an org. |
| `decideRefund(id, body?, opts?)` | `POST /api/v1/organizer/refunds/:id/decide` | Approve or deny a refund request. |
| `reports(orgId, query?, opts?)` | `GET /api/v1/organizer/orgs/:orgId/reports` | List reports filed against an org (read-only). |
| `resolveReport(id, body?, opts?)` | `POST /api/v1/organizer/reports/:id/resolve` | Resolve a report (admins only; organizers receive 403). |
| `messages(orgId, query?, opts?)` | `GET /api/v1/organizer/orgs/:orgId/messages` | Org-wide buyer message threads. |
| `overview(orgId, query?, opts?)` | `GET /api/v1/organizer/orgs/:orgId/overview` | Org dashboard overview. |
| `referral(orgId, query?, opts?)` | `GET /api/v1/organizer/orgs/:orgId/referral` | Referral program summary + commissions. |
| `notifications(orgId, query?, opts?)` | `GET /api/v1/organizer/orgs/:orgId/notifications` | Org notifications. |
| `fundWallet(orgId, body?, opts?)` | `POST /api/v1/organizer/wallets/org/:orgId/fund` | Top up the org wallet (returns a payment intent). |
| `fundWalletStatus(orgId, orderId, query?, opts?)` | `GET /api/v1/organizer/wallets/org/:orgId/fund/:orderId` | Poll a wallet top-up payment. |
| `payoutDetails(orgId, query?, opts?)` | `GET /api/v1/organizer/orgs/:orgId/payout-details` | Stored payout destinations. |
| `updatePayoutDetails(orgId, currency, body?, opts?)` | `PUT /api/v1/organizer/orgs/:orgId/payout-details/:currency` | Set payout details for a currency. |
| `ticketMessage(ticketId, body?, opts?)` | `POST /api/v1/organizer/tickets/:ticketId/message` | Reply to a buyer on a ticket thread. |
| `messageThread(threadId, query?, opts?)` | `GET /api/v1/organizer/messages/:threadId` | A single org message thread. |

### `client.wallets` Wallets

Organization wallet balances and ledger.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query?, opts?)` | `GET /api/v1/organizer/wallets` | List wallets the caller can see. |
| `get(orgId, query?, opts?)` | `GET /api/v1/organizer/wallets/org/:orgId` | An org's wallet balances (per currency). |
| `bootstrap(orgId, body?, opts?)` | `POST /api/v1/organizer/wallets/org/:orgId/bootstrap` | Provision an org's wallet. |
| `transactions(orgId, query?, opts?)` | `GET /api/v1/organizer/wallets/org/:orgId/transactions` | Wallet ledger transactions. |

### `client.scannerTokens` Scanner tokens

Manage short-lived gate-scanner tokens for staff devices.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(orgId, query?, opts?)` | `GET /api/v1/organizer/scanner-tokens/org/:orgId` | List scanner tokens. |
| `create(orgId, body?, opts?)` | `POST /api/v1/organizer/scanner-tokens/org/:orgId` | Mint a scanner token. |
| `update(orgId, tokenId, body?, opts?)` | `PUT /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId` | Update a scanner token. |
| `delete(orgId, tokenId, body?, opts?)` | `DELETE /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId` | Revoke a scanner token. |
| `reissue(orgId, tokenId, body?, opts?)` | `POST /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId/reissue` | Reissue a scanner token's secret. |
| `metrics(orgId, tokenId, query?, opts?)` | `GET /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId/metrics` | Usage metrics for a scanner token. |

### `client.integrations` Integrations

API keys, MCP tokens, and integration usage metrics.

| Method | Endpoint | Description |
| --- | --- | --- |
| `metrics(orgId, query?, opts?)` | `GET /api/v1/organizer/integrations/org/:orgId/metrics` | Integration usage metrics. |
| `apiCalls(orgId, query?, opts?)` | `GET /api/v1/organizer/integrations/org/:orgId/api-calls` | Recent API call log. |
| `mcpCalls(orgId, query?, opts?)` | `GET /api/v1/organizer/integrations/org/:orgId/mcp-calls` | Recent MCP tool-call log. |
| `listApiKeys(orgId, query?, opts?)` | `GET /api/v1/organizer/integrations/org/:orgId/api-keys` | List API keys. |
| `createApiKey(orgId, body?, opts?)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys` | Mint an API key (plaintext returned once). |
| `updateApiKey(orgId, keyId, body?, opts?)` | `PUT /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Update an API key (scopes, status, allowlist). |
| `rotateApiKey(orgId, keyId, body?, opts?)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId/rotate` | Rotate an API key's secret. |
| `deleteApiKey(orgId, keyId, body?, opts?)` | `DELETE /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Revoke an API key. |
| `listMcpTokens(orgId, query?, opts?)` | `GET /api/v1/organizer/integrations/org/:orgId/mcp-tokens` | List MCP tokens. |
| `createMcpToken(orgId, body?, opts?)` | `POST /api/v1/organizer/integrations/org/:orgId/mcp-tokens` | Mint an MCP token. |
| `updateMcpToken(orgId, tokenId, body?, opts?)` | `PUT /api/v1/organizer/integrations/org/:orgId/mcp-tokens/:tokenId` | Update an MCP token. |
| `deleteMcpToken(orgId, tokenId, body?, opts?)` | `DELETE /api/v1/organizer/integrations/org/:orgId/mcp-tokens/:tokenId` | Revoke an MCP token. |

### `client.eventCustomization` Event customization

Per-event white-label theming.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get(eventId, query?, opts?)` | `GET /api/v1/organizer/event-customization/:eventId` | Get an event's customization. |
| `update(eventId, body?, opts?)` | `PUT /api/v1/organizer/event-customization/:eventId` | Update an event's customization. |

### `client.growth` Growth

Comp tickets, CSV import, broadcasts and attendee tags.

| Method | Endpoint | Description |
| --- | --- | --- |
| `mintComps(eventId, body?, opts?)` | `POST /api/v1/organizer/growth/events/:eventId/comps` | Issue complimentary tickets. |
| `importCompsCsv(eventId, body?, opts?)` | `POST /api/v1/organizer/growth/events/:eventId/comps/import-csv` | Bulk-issue comps from CSV. |
| `listComps(eventId, query?, opts?)` | `GET /api/v1/organizer/growth/events/:eventId/comps` | List issued comps. |
| `resendComp(eventId, ticketId, body?, opts?)` | `POST /api/v1/organizer/growth/events/:eventId/comps/:ticketId/resend` | Resend a comp ticket email. |
| `broadcastEvent(eventId, body?, opts?)` | `POST /api/v1/organizer/growth/events/:eventId/broadcast` | Broadcast to an event's attendees. |
| `broadcastOrg(orgId, body?, opts?)` | `POST /api/v1/organizer/growth/orgs/:orgId/broadcast` | Broadcast to an org's followers/subscribers. |
| `listBroadcasts(orgId, query?, opts?)` | `GET /api/v1/organizer/growth/orgs/:orgId/broadcasts` | List sent broadcasts. |
| `addTags(body?, opts?)` | `POST /api/v1/organizer/growth/tags` | Tag attendees (body: orgId, ticketIds, tag). |
| `removeTag(body?, opts?)` | `DELETE /api/v1/organizer/growth/tags` | Remove a tag (body: orgId, ticketId, tag). |
| `listTags(orgId, query?, opts?)` | `GET /api/v1/organizer/growth/orgs/:orgId/tags` | List tags for an org. |
| `tagAttendees(orgId, tag, query?, opts?)` | `GET /api/v1/organizer/growth/orgs/:orgId/tags/:tag/attendees` | List attendees with a given tag. |

### `client.publicEvents` Public events (vanity)

Public org/event read endpoints used by hosted pages and white-label sites.

| Method | Endpoint | Description |
| --- | --- | --- |
| `getBySlug(slug, query?, opts?)` | `GET /api/v1/public/events/:slug` | Public event by slug. |
| `orgEvents(orgId, query?, opts?)` | `GET /api/v1/public/events/orgs/by/:orgId` | An organization's public events. |
| `getByOrgEvent(orgId, eventId, query?, opts?)` | `GET /api/v1/public/events/by/:orgId/:eventId` | Public event by org + event id. |
| `getById(eventId, query?, opts?)` | `GET /api/v1/public/events/by-id/:eventId` | Public event by id. |
| `preview(eventId, query?, opts?)` | `GET /api/v1/public/events/preview/:eventId` | Draft event preview (token-gated). |

### `client.site` Public site

Sitemap, newsletter signup and platform status.

| Method | Endpoint | Description |
| --- | --- | --- |
| `sitemap(query?, opts?) → BinaryResponse` | `GET /api/v1/public/sitemap.xml` | Sitemap XML (application/xml bytes). |
| `newsletterStart(body?, opts?)` | `POST /api/v1/public/newsletter/start` | Begin newsletter double-opt-in. |
| `newsletterConfirm(body?, opts?)` | `POST /api/v1/public/newsletter` | Confirm a newsletter subscription. |
| `status(query?, opts?)` | `GET /api/v1/public/status` | Platform status + incidents. |

### `client.community` Community

Verified-attendee reviews, organizer follows and event waitlists.

| Method | Endpoint | Description |
| --- | --- | --- |
| `submitReview(body?, opts?)` | `POST /api/v1/community/reviews` | Submit a verified-attendee review (ticketCode + email prove ownership). |
| `orgReviews(orgId, query?, opts?)` | `GET /api/v1/community/orgs/:orgId/reviews` | Published reviews for an org. |
| `eventReviews(eventId, query?, opts?)` | `GET /api/v1/community/events/:eventId/reviews` | Published reviews for an event. |
| `follow(orgId, body?, opts?)` | `POST /api/v1/community/orgs/:orgId/follow` | Follow an organizer. |
| `unfollow(token, query?, opts?)` | `GET /api/v1/community/unfollow/:token` | Unsubscribe via emailed token. |
| `joinWaitlist(eventId, body?, opts?)` | `POST /api/v1/community/events/:eventId/waitlist` | Join an event waitlist. |
| `acceptWaitlist(id, token, query?, opts?)` | `GET /api/v1/community/waitlist/accept/:id/:token` | Accept a waitlist offer via emailed token. |
| `manageReviews(orgId, query?, opts?)` | `GET /api/v1/community/orgs/:orgId/reviews/manage` | Organizer view of all reviews (incl. pending). |
| `replyReview(id, body?, opts?)` | `POST /api/v1/community/reviews/:id/reply` | Organizer reply to a review. |
| `setReviewStatus(id, body?, opts?)` | `POST /api/v1/community/reviews/:id/status` | Publish/hide a review. |
| `followers(orgId, query?, opts?)` | `GET /api/v1/community/orgs/:orgId/followers` | List an org's followers. |
| `removeFollower(orgId, followerId, body?, opts?)` | `DELETE /api/v1/community/orgs/:orgId/followers/:followerId` | Remove a follower. |
| `eventWaitlist(eventId, query?, opts?)` | `GET /api/v1/community/events/:eventId/waitlist` | List an event's waitlist. |
| `offerWaitlist(eventId, body?, opts?)` | `POST /api/v1/community/events/:eventId/waitlist/offer` | Offer spots to waitlisted attendees. |

### `client.track` Tracking

Lightweight page-view tracking.

| Method | Endpoint | Description |
| --- | --- | --- |
| `view(body?, opts?)` | `POST /api/v1/track/view` | Record a page view. |

### `client.webhooks` Webhooks

Register webhook endpoints and inspect/replay deliveries. (Use client.webhooks.verify() to validate inbound signatures.)

| Method | Endpoint | Description |
| --- | --- | --- |
| `catalog(query?, opts?)` | `GET /api/v1/webhooks/catalog` | List subscribable event types. |
| `list(query?, opts?)` | `GET /api/v1/webhooks` | List registered webhook endpoints. |
| `create(body?, opts?)` | `POST /api/v1/webhooks` | Register a webhook endpoint. |
| `update(id, body?, opts?)` | `PUT /api/v1/webhooks/:id` | Update a webhook endpoint. |
| `rotateSecret(id, body?, opts?)` | `POST /api/v1/webhooks/:id/rotate-secret` | Rotate a webhook signing secret. |
| `delete(id, body?, opts?)` | `DELETE /api/v1/webhooks/:id` | Delete a webhook endpoint. |
| `deliveries(id, query?, opts?)` | `GET /api/v1/webhooks/:id/deliveries` | List delivery attempts for an endpoint. |
| `test(id, body?, opts?)` | `POST /api/v1/webhooks/:id/test` | Send a test event to an endpoint. |
| `replay(id, body?, opts?)` | `POST /api/v1/webhooks/deliveries/:id/replay` | Replay a past delivery. |

### `client.whiteLabel` White-label

Self-contained white-label surface: events, ticket types, customization, orders, tickets and check-in under one integration.

| Method | Endpoint | Description |
| --- | --- | --- |
| `me(query?, opts?)` | `GET /api/v1/white-label/me` | The integration's white-label context. |
| `events(query?, opts?)` | `GET /api/v1/white-label/events` | List white-label events. |
| `createEvent(body?, opts?)` | `POST /api/v1/white-label/events` | Create a white-label event. |
| `getEvent(id, query?, opts?)` | `GET /api/v1/white-label/events/:id` | White-label event detail. |
| `updateEvent(id, body?, opts?)` | `PUT /api/v1/white-label/events/:id` | Update a white-label event. |
| `deleteEvent(id, body?, opts?)` | `DELETE /api/v1/white-label/events/:id` | Delete a white-label event. |
| `ticketTypes(eventId, query?, opts?)` | `GET /api/v1/white-label/events/:eventId/ticket-types` | List ticket types. |
| `createTicketType(eventId, body?, opts?)` | `POST /api/v1/white-label/events/:eventId/ticket-types` | Create a ticket type. |
| `getCustomization(eventId, query?, opts?)` | `GET /api/v1/white-label/events/:eventId/customization` | Get event customization. |
| `updateCustomization(eventId, body?, opts?)` | `PUT /api/v1/white-label/events/:eventId/customization` | Update event customization. |
| `orders(query?, opts?)` | `GET /api/v1/white-label/orders` | List white-label orders. |
| `tickets(query?, opts?)` | `GET /api/v1/white-label/tickets` | List white-label tickets. |
| `checkinScan(body?, opts?)` | `POST /api/v1/white-label/checkin/scan` | Validate a ticket at a white-label gate. |
| `checkinStats(eventId, query?, opts?)` | `GET /api/v1/white-label/checkin/stats/:eventId` | White-label check-in stats. |
| `wallets(query?, opts?)` | `GET /api/v1/white-label/wallets` | White-label wallet balances. |

### `client.support` Support

Support tickets and threaded messages.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query?, opts?)` | `GET /api/v1/support` | List support tickets. |
| `create(body?, opts?)` | `POST /api/v1/support` | Open a support ticket. |
| `get(id, query?, opts?)` | `GET /api/v1/support/:id` | Support ticket detail. |
| `sendMessage(id, body?, opts?)` | `POST /api/v1/support/:id/messages` | Reply on a support ticket. |

### `client.util` Utilities

Helper endpoints.

| Method | Endpoint | Description |
| --- | --- | --- |
| `resolveCoords(body?, opts?)` | `POST /api/v1/util/resolve-coords` | Resolve an address to coordinates. |

<!-- END ENDPOINTS -->
