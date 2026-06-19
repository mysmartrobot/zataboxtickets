# zatabox/zatabox PHP SDK

Official **PHP SDK** for the [Zatabox Tickets](https://zatabox.com) REST API the
white-label event-ticketing platform. A small, dependency-free client over
`https://api.zatabox.com/api/v1` that handles auth, sandbox routing, idempotency,
retries, pagination, live (SSE) streaming and webhook verification.

- **Zero Composer dependencies** bundled `curl`, `json` and `hash` extensions.
- **Complete** every one of the **78 REST endpoints** is a method.
- **Generated, never drifts** emitted from the canonical
  [`endpoints.json`](../spec/endpoints.json) spec.
- **PHP 7.0+**.

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
- [Troubleshooting & FAQ](#troubleshooting--faq)
- [Full endpoint reference](#full-endpoint-reference)
- [Versioning & support](#versioning--support)
- [License](#license)

---

## Requirements

- **PHP 7.0 or newer** with the `curl`, `json` and `hash` extensions (all bundled with
  a standard PHP build).
- A Zatabox **API key** (`vt_live_…` / `vt_test_…`), a **portal JWT**, or an **MCP
  token**. Mint API keys in the organizer portal → Integrations, or the
  [sandbox console](https://tester.zatabox.com).

## Installation

This SDK is **distributed via GitHub** it is not published to Packagist. The package
lives in the `php/` directory of
[`mysmartrobot/zataboxtickets`](https://github.com/mysmartrobot/zataboxtickets).

**Option A Composer path repository.** Add the repo as a submodule (or clone it),
then point Composer at the `php/` subdirectory:

```bash
git submodule add https://github.com/mysmartrobot/zataboxtickets.git vendor-src/zataboxtickets
```

```jsonc
// composer.json
{
  "repositories": [
    { "type": "path", "url": "vendor-src/zataboxtickets/php" }
  ],
  "require": { "zatabox/zatabox": "*" }
}
```

```bash
composer install   # autoloads Zatabox\Client, Zatabox\ZataboxError, …
```

**Option B no Composer.** Clone the repo and require the bundled autoloader (it pulls
in the resource classes and the client):

```php
require __DIR__ . '/vendor-src/zataboxtickets/php/src/autoload.php';
```

Either way:

```php
use Zatabox\Client;
use Zatabox\ZataboxError;
```

## Quick start

```php
use Zatabox\Client;
use Zatabox\ZataboxError;

// A vt_test_ key auto-routes to the sandbox; a vt_live_ key to production.
$z = new Client(['apiKey' => getenv('ZATABOX_API_KEY')]);

try {
    $event = $z->organizer->createEvent([
        'title' => 'Warehouse Sessions 004',
        'category' => 'music',
        'startDate' => '2026-08-22T20:00:00Z',
        'endDate' => '2026-08-23T02:00:00Z',
        'timezone' => 'Africa/Lagos',
        'venueType' => 'physical',
        'venueCity' => 'Lagos',
        'capacity' => 450,
    ]);
    $z->organizer->createTicket($event['id'], [
        'name' => 'General Admission', 'type' => 'general', 'price' => 5000,
        'currency' => 'NGN', 'quantityTotal' => 450,
        'saleStart' => '2026-07-01T00:00:00Z', 'saleEnd' => '2026-08-22T20:00:00Z',
    ]);
    $z->organizer->publishEvent($event['id']);
    echo $z->events->get($event['slug'])['status'];
} catch (ZataboxError $e) {
    fwrite(STDERR, "{$e->errorCode} {$e->getMessage()} {$e->requestId}\n");
}
```

## Authentication

The SDK forwards one `Authorization: Bearer <token>` header. Three ways to authenticate:

```php
new Client(['apiKey' => 'vt_live_...']);   // scoped API key (prefix selects env)
new Client(['apiKey' => 'vt_test_...']);   // sandbox (auto-routed)
new Client(['bearerToken' => 'eyJ...']);   // portal JWT or vt_mcp_ token
new Client(['apiKey' => 'vt_test_...', 'baseUrl' => 'http://localhost:4100']);
```

### Passwordless buyer login

```php
$anon = new Client(['bearerToken' => 'unused', 'baseUrl' => 'https://api.zatabox.com']);
$anon->auth->requestToken(['email' => 'fan@example.com']);              // emails a code
$session = $anon->auth->exchangeToken(['email' => 'fan@example.com', 'code' => '123456']);

$buyer = new Client(['bearerToken' => $session['accessToken']]);
$tickets = $buyer->users->tickets();
```

### Refreshing & swapping tokens

```php
$next = $buyer->auth->refresh(['refreshToken' => $session['refreshToken']]);
$buyer->setBearerToken($next['accessToken']);
```

### API-key scopes

Keys can be minted with least-privilege scopes: `events:read`, `events:write`,
`tickets:read`, `tickets:write`, `orders:read`, `orders:write`, `attendees:read`,
`attendees:write`, `checkin:write`, `payouts:read`, `payouts:write`, `webhooks:manage`,
`analytics:read`, `*`. A call beyond a key's scopes returns `403 INSUFFICIENT_SCOPE`.

## Sandbox / test mode

`vt_test_` keys auto-route to the Zatabox **sandbox** at `https://sandbox.zatabox.com`
a full mirror of the API with no real charges, emails or SMS.

```php
new Client(['apiKey' => 'vt_test_...']);   // → sandbox.zatabox.com
new Client(['apiKey' => 'vt_live_...']);   // → api.zatabox.com
new Client(['apiKey' => 'vt_test_...', 'baseUrl' => 'http://localhost:4100']);
```

Mint/rotate `vt_test_` keys, watch **live request logs**, see usage and browse the
endpoint catalog in the **sandbox console at https://tester.zatabox.com** (sign in
with your production account). A test key used against production or vice-versa 
returns `403 WRONG_ENV`.

## Client configuration

```php
new Client([
    'apiKey' => 'vt_live_...',   // or 'bearerToken'
    'baseUrl' => null,           // override the auto-resolved host
    'timeout' => 30,             // per-request timeout (seconds)
    'maxRetries' => 2,           // retries for 5xx / network / timeout
    'userAgent' => null,         // defaults to zatabox-php/<version>
]);
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `apiKey` | `string` | | `vt_live_…` / `vt_test_…`; test keys auto-route to the sandbox. |
| `bearerToken` | `string` | | Portal JWT / `vt_mcp_…`. One credential is required. |
| `baseUrl` | `string` | resolved from key | Explicit API origin; wins over prefix routing. |
| `timeout` | `int` | `30` | Per-request timeout (seconds). |
| `maxRetries` | `int` | `2` | Retries for `5xx`/network/timeout (never `4xx`). |
| `userAgent` | `string` | `zatabox-php/<version>` | Overrides the `User-Agent` header. |

## How methods map to endpoints

`$client-><namespace>-><method>(...)`. Namespaces are camelCase:

`auth`, `events`, `organizer`, `eventCustomization`, `tickets`, `orders`, `payments`,
`checkin`, `community`, `growth`, `users`, `integrations`, `webhooks`.

Argument order:

```
method($pathParam1, $pathParam2, …, $payload = null, array $opts = [])
```

- **Path params** first (URL-encoded for you).
- **Reads** take an optional `$query` array; **writes** take an optional `$body` array.
- **`$opts`** carries `'idempotencyKey'`, `'headers'`, and (for writes) an extra
  `'query'`.

```php
$z->events->list(['q' => 'jazz', 'city' => 'Lagos', 'limit' => 20]);        // query
$z->events->get('warehouse-sessions-004');                                  // path param
$z->organizer->updateSchedule($eventId, $sessionId, ['sessionTitle' => 'Keynote']);  // two params + body
$z->orders->create($cart, ['idempotencyKey' => $myUuid]);                   // body + opts
```

## Responses

The API wraps success as `{ "success", "data", "meta" }`; the SDK **returns `data`
directly** as an associative array. Pagination cursors and `request_id` live inside.

```php
$page = $z->events->list(['limit' => 20]);   // e.g. ['items' => [...], 'pagination' => [...]]
```

## Error handling

Any non-2xx (and network/timeout/webhook failures) throws **`Zatabox\ZataboxError`**.
The stable machine code is in `$e->errorCode` (the base `getCode()` is unused):

```php
use Zatabox\ZataboxError;

try {
    $z->orders->create(['items' => $items]);
} catch (ZataboxError $e) {
    $e->errorCode;        // 'TICKET_SOLD_OUT'
    $e->status;           // 409 (0 for network errors)
    $e->getMessage();     // human-readable
    $e->requestId;        // 'req_01J9...'
    $e->details;          // ['ticketTypeId' => 'tkt_8f2k']
}
```

### Common error codes

| `errorCode` | `status` | Meaning |
| --- | --- | --- |
| `VALIDATION_ERROR` | 400 | Body failed validation. |
| `UNAUTHORIZED` / `INVALID_TOKEN` | 401 | Missing/expired credential. |
| `WRONG_ENV` | 403 | Test key on production or vice-versa. |
| `INSUFFICIENT_SCOPE` | 403 | Key lacks the route's scope. |
| `NOT_FOUND` | 404 | No such resource. |
| `CONFLICT` / `IDEMPOTENCY_KEY_REUSED` | 409 | Unique/idempotency clash. |
| `TICKET_SOLD_OUT` | 409 | Inventory exhausted. |
| `RATE_LIMITED` | 429 | Throttled; see `$e->details['retryAfter']`. |
| `INTERNAL_ERROR` | 500 | Server error (auto-retried). |
| `NETWORK_ERROR` | 0 | Connection/timeout after retries (SDK-side). |
| `MISSING_SIGNATURE` / `INVALID_SIGNATURE` / `SIGNATURE_EXPIRED` | | webhook verify failures. |

## Idempotency

Every write auto-sends an `Idempotency-Key` (fresh UUIDv4). The server caches the result
for 24h replaying the same key + body returns the original; the same key with a
different body returns `409 IDEMPOTENCY_KEY_REUSED`. Pass your own to make a retry safe:

```php
$key = bin2hex(random_bytes(16));
$z->orders->create($cart, ['idempotencyKey' => $key]);
$z->orders->create($cart, ['idempotencyKey' => $key]);  // safe retry no double charge
```

## Retries, timeouts & networking

- **Timeouts** bounded by `timeout` (seconds); treated as a retryable failure.
- **Retries** `5xx`/network/timeout retried up to `maxRetries` with exponential
  backoff; `4xx` never retried.
- **Rate limits** `429` throws `ZataboxError` with `errorCode` `RATE_LIMITED` and
  `details['retryAfter']`.

## Pagination

```php
foreach ($z->paginate([$z->events, 'list'], ['q' => 'jazz', 'limit' => 50]) as $page) {
    foreach ($page['items'] as $ev) {
        echo $ev['id'];
    }
}
```

`paginate(callable, query)` accepts any cursor-paginated list (e.g. `[$z->events,
'list']`, `[$z->users, 'tickets']`) and follows the cursor across both response shapes.
To page manually:

```php
$cursor = null;
do {
    $page = $z->events->list(['limit' => 50, 'cursor' => $cursor]);
    // ...
    $cursor = $page['pagination']['cursor'] ?? $page['nextCursor'] ?? null;
} while ($cursor);
```

## Live check-in stream (SSE)

`$z->checkin->liveUrl($eventId)` returns the stream URL; consume it with an SSE client
(passing `Authorization: Bearer <key>`).

## Verifying inbound webhooks

Signature header: `X-Zatabox-Signature: t=<unix>,v1=<hex-hmac-sha256>`; the signed
payload is `<t>.<raw_body>`, HMAC-SHA256 with your endpoint secret (constant-time, 5-min
tolerance). **Verify the raw request body.**

```php
$raw = file_get_contents('php://input');
try {
    $event = $z->webhooks->verify($raw, $_SERVER['HTTP_X_ZATABOX_SIGNATURE'], getenv('ZATABOX_WEBHOOK_SECRET'));
    if ($event['type'] === 'order.paid') {
        // fulfil
    }
    http_response_code(200);
} catch (\Zatabox\ZataboxError $e) {
    http_response_code(400);
}
```

## End-to-end recipes

```php
// Sell a ticket (guest checkout)
$order = $z->orders->create(['items' => [['ticketTypeId' => $tt, 'quantity' => 2]], 'guestEmail' => 'fan@example.com']);
$intent = $z->orders->pay($order['id'], ['provider' => 'paystack']);
$paid = $z->payments->verify(['orderId' => $order['id']]);

// Check in at the gate
$res = $z->checkin->scan(['qrData' => $qr, 'gateName' => 'Main', 'deviceId' => $device]);
$stats = $z->checkin->stats($eventId);
```

## Troubleshooting & FAQ

- **`InvalidArgumentException: pass apiKey or bearerToken`** provide a credential.
- **`403 WRONG_ENV`** match the key prefix to the environment.
- **`409 IDEMPOTENCY_KEY_REUSED`** reused a key with a different body.
- **Webhook `INVALID_SIGNATURE`** verify `php://input`, not `$_POST`.
- **A list looks short** iterate with `$z->paginate(...)`.

## Versioning & support

SemVer; version is `Zatabox\Client::VERSION`. API base `https://api.zatabox.com/api/v1`
· Docs <https://zatabox.com/docs> · developers@zatabox.com.

## License

MIT

---

## Full endpoint reference

<!-- BEGIN ENDPOINTS (generated by scripts/generate.mjs do not edit) -->

The SDK exposes **78 endpoints** across **13 namespaces**. Every method is listed below with its idiomatic signature, the underlying HTTP route, and what it does. Path parameters are positional; reads take an optional query map and writes take an optional body, both followed by a call-options bag.

### `$client->auth` Auth

Account registration, password + passwordless sign-in, 2FA login and token refresh.

| Method | Endpoint | Description |
| --- | --- | --- |
| `register($body = null, array $opts = [])` | `POST /api/v1/auth/register` | Register an account; returns the user plus an accessToken/refreshToken pair. |
| `login($body = null, array $opts = [])` | `POST /api/v1/auth/login` | Log in with email + password; returns a JWT pair (or a 2FA challenge). |
| `loginVerify2fa($body = null, array $opts = [])` | `POST /api/v1/auth/2fa-verify` | Complete a 2FA login challenge; returns the JWT pair. |
| `requestToken($body = null, array $opts = [])` | `POST /api/v1/auth/token/request` | Passwordless: email a buyer a 6-digit login code. |
| `exchangeToken($body = null, array $opts = [])` | `POST /api/v1/auth/token/exchange` | Passwordless: exchange email + 6-digit code for a JWT pair. |
| `refresh($body = null, array $opts = [])` | `POST /api/v1/auth/refresh` | Refresh an expired access token (rotates the refresh token). |
| `logout($body = null, array $opts = [])` | `POST /api/v1/auth/logout` | Revoke a refresh token. |

### `$client->events` Events (Public)

Public event discovery and read, plus external ticket issuance.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list($query = null, array $opts = [])` | `GET /api/v1/events` | List and search published public events (cursor-paginated). |
| `get($slug, $query = null, array $opts = [])` | `GET /api/v1/events/:slug` | Event detail by slug (organizer info, schedule, active ticket types). |
| `tickets($id, $query = null, array $opts = [])` | `GET /api/v1/events/:id/tickets` | List an event's ticket types with live availability. |
| `issue($eventId, $body = null, array $opts = [])` | `POST /api/v1/events/:eventId/issue` | Issue tickets you sold elsewhere (developer-handled payment; 3% wallet fee on paid tickets). |

### `$client->organizer` Organizer

Organizer surface: organization read, events, ticket types, schedule sessions, seating sections and promo codes.

| Method | Endpoint | Description |
| --- | --- | --- |
| `getOrganization($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/organizations/:id` | Get organization details and per-currency wallet balances. |
| `createEvent($body = null, array $opts = [])` | `POST /api/v1/organizer/events` | Create a draft event. |
| `updateEvent($id, $body = null, array $opts = [])` | `PUT /api/v1/organizer/events/:id` | Partial-update an event. |
| `publishEvent($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/events/:id/publish` | Publish a draft event. |
| `unpublishEvent($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/events/:id/unpublish` | Unpublish a published event back to draft. |
| `deleteEvent($id, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/events/:id` | Cancel an event. |
| `createTicket($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/events/:id/tickets` | Create a ticket type. |
| `schedule($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/events/:id/schedule` | List schedule sessions (running order). |
| `createSchedule($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/events/:id/schedule` | Add a schedule session. |
| `updateSchedule($id, $sessionId, $body = null, array $opts = [])` | `PUT /api/v1/organizer/events/:id/schedule/:sessionId` | Update a schedule session. |
| `deleteSchedule($id, $sessionId, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/events/:id/schedule/:sessionId` | Delete a schedule session. |
| `sections($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/events/:id/sections` | List seating/capacity sections. |
| `createSection($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/events/:id/sections` | Add a seating section. |
| `updateSection($id, $sectionId, $body = null, array $opts = [])` | `PUT /api/v1/organizer/events/:id/sections/:sectionId` | Update a seating section. |
| `deleteSection($id, $sectionId, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/events/:id/sections/:sectionId` | Delete a seating section. |
| `promoCodes($query = null, array $opts = [])` | `GET /api/v1/organizer/promo-codes` | List promo codes (optionally filtered by event). |
| `createPromoCode($body = null, array $opts = [])` | `POST /api/v1/organizer/promo-codes` | Create a promo code. |
| `updatePromoCode($id, $body = null, array $opts = [])` | `PUT /api/v1/organizer/promo-codes/:id` | Update a promo code. |
| `deletePromoCode($id, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/promo-codes/:id` | Delete or disable a promo code. |

### `$client->eventCustomization` Event page customization

Per-event public-page theming and the “Good to know” FAQ.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/event-customization/:id` | Get an event's page customization (theme, layout, FAQ, SEO). |
| `update($id, $body = null, array $opts = [])` | `PUT /api/v1/organizer/event-customization/:id` | Update an event's page customization (incl. the FAQ list). |

### `$client->tickets` Tickets

Checkout-time ticket helpers.

| Method | Endpoint | Description |
| --- | --- | --- |
| `validatePromo($body = null, array $opts = [])` | `POST /api/v1/tickets/promo/validate` | Validate a promo code against a cart (read-only preview, does not consume a use). |

### `$client->orders` Orders

Carted checkout: create, read, pay, cancel.

| Method | Endpoint | Description |
| --- | --- | --- |
| `create($body = null, array $opts = [])` | `POST /api/v1/orders` | Create an order (guest checkout needs only name + email). |
| `get($id, $query = null, array $opts = [])` | `GET /api/v1/orders/:id` | Get an order (pass ?token for guest reads). |
| `pay($id, $body = null, array $opts = [])` | `POST /api/v1/orders/:id/pay` | Initiate payment (provider: nowpayments \| paystack \| flutterwave). |
| `cancel($id, $body = null, array $opts = [])` | `POST /api/v1/orders/:id/cancel` | Cancel an unpaid order and release held inventory. |

### `$client->payments` Payments

Verify charges, read payment status, list crypto coins.

| Method | Endpoint | Description |
| --- | --- | --- |
| `verify($body = null, array $opts = [])` | `POST /api/v1/payments/verify` | Actively verify a payment with the provider and issue tickets (idempotent, poll-safe). |
| `get($orderId, $query = null, array $opts = [])` | `GET /api/v1/payments/:orderId` | Read payment/order status and attempts (read-only). |
| `cryptoCurrencies($query = null, array $opts = [])` | `GET /api/v1/payments/crypto/currencies` | List supported NOWPayments crypto coins (for the payCurrency value). |

### `$client->checkin` Check-in

Gate scanning, offline manifests + sync, live stats.

| Method | Endpoint | Description |
| --- | --- | --- |
| `scan($body = null, array $opts = [])` | `POST /api/v1/checkin/scan` | Validate a QR, barcode or 6-character door code at the gate. |
| `manual($id, $body = null, array $opts = [])` | `POST /api/v1/checkin/event/:id/manual` | Manually check in a typed ticket code. |
| `manifest($id, $query = null, array $opts = [])` | `GET /api/v1/checkin/event/:id/manifest` | Hashed guest-list manifest for offline scanning (pass ?since for a delta). |
| `batch($body = null, array $opts = [])` | `POST /api/v1/checkin/batch` | Sync up to 500 queued offline scans. |
| `stats($id, $query = null, array $opts = [])` | `GET /api/v1/checkin/event/:id/stats` | Check-in totals, capacity %, entry rate and per-gate breakdown. |
| `gate($id, $gate, $query = null, array $opts = [])` | `GET /api/v1/checkin/event/:id/gate/:gate` | Per-gate check-in stats slice. |
| `liveUrl($id, $query = null)` | `GET /api/v1/checkin/event/:id/live` | Server-Sent Events stream a stats snapshot every 2 seconds. |

### `$client->community` Community

Verified-attendee reviews, organizer follows/subscribers and event waitlists.

| Method | Endpoint | Description |
| --- | --- | --- |
| `submitReview($body = null, array $opts = [])` | `POST /api/v1/community/reviews` | Review an event (checked-in ticket holders only; ticketCode + email prove attendance). |
| `follow($orgId, $body = null, array $opts = [])` | `POST /api/v1/community/orgs/:orgId/follow` | Follow an organizer (subscribe to new-event announcements). |
| `followers($orgId, $query = null, array $opts = [])` | `GET /api/v1/community/orgs/:orgId/followers` | List an organizer's subscribers (organizer auth). |
| `removeFollower($orgId, $followerId, $body = null, array $opts = [])` | `DELETE /api/v1/community/orgs/:orgId/followers/:followerId` | Remove a subscriber (organizer auth). |
| `joinWaitlist($eventId, $body = null, array $opts = [])` | `POST /api/v1/community/events/:eventId/waitlist` | Join an event waitlist (offers fire on cancellations). |

### `$client->growth` Growth (Organizer)

Comp tickets, CSV import, broadcasts and attendee tags.

| Method | Endpoint | Description |
| --- | --- | --- |
| `mintComps($eventId, $body = null, array $opts = [])` | `POST /api/v1/organizer/growth/events/:eventId/comps` | Bulk-mint and email complimentary tickets. |
| `importCompsCsv($eventId, $body = null, array $opts = [])` | `POST /api/v1/organizer/growth/events/:eventId/comps/import-csv` | Import attendees (comp tickets) from CSV. |
| `broadcastEvent($eventId, $body = null, array $opts = [])` | `POST /api/v1/organizer/growth/events/:eventId/broadcast` | Email a broadcast to an event's attendees (replies thread to the organizer inbox). |
| `addTags($body = null, array $opts = [])` | `POST /api/v1/organizer/growth/tags` | Tag attendees (additive; powers broadcast filters and CRM segments). |
| `removeTag($body = null, array $opts = [])` | `DELETE /api/v1/organizer/growth/tags` | Remove an attendee tag. |

### `$client->users` Buyers

The authenticated buyer: profile, ticket wallet, data export, refunds, reports and organizer messaging.

| Method | Endpoint | Description |
| --- | --- | --- |
| `me($query = null, array $opts = [])` | `GET /api/v1/users/me` | Current buyer profile. |
| `tickets($query = null, array $opts = [])` | `GET /api/v1/users/me/tickets` | The buyer's ticket wallet (cursor-paginated). |
| `export($query = null, array $opts = [])` | `GET /api/v1/users/me/export` | GDPR data export one JSON download of everything on the account. |
| `createRefund($body = null, array $opts = [])` | `POST /api/v1/users/me/refunds` | Request a refund for a ticket. |
| `createReport($body = null, array $opts = [])` | `POST /api/v1/users/me/reports` | File a report against an event or organizer. |
| `messages($query = null, array $opts = [])` | `GET /api/v1/users/me/messages` | The buyer's message threads with organizers. |
| `sendTicketMessage($ticketId, $body = null, array $opts = [])` | `POST /api/v1/users/me/tickets/:ticketId/message` | Message the organizer about a ticket (rate-limited). |

### `$client->integrations` API keys

Manage your organization's own API keys (organizer owner/admin auth).

| Method | Endpoint | Description |
| --- | --- | --- |
| `createApiKey($orgId, $body = null, array $opts = [])` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys` | Create an API key (plaintext secret returned exactly once). |
| `listApiKeys($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/integrations/org/:orgId/api-keys` | List API keys (prefixes and metadata only, never the secret). |
| `updateApiKey($orgId, $keyId, $body = null, array $opts = [])` | `PUT /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Update a key (rename, pause, re-scope). |
| `rotateApiKey($orgId, $keyId, $body = null, array $opts = [])` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId/rotate` | Rotate a key's secret (new secret returned once; old one invalidated). |
| `deleteApiKey($orgId, $keyId, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Revoke an API key. |

### `$client->webhooks` Webhooks

Register webhook endpoints, manage secrets, inspect and replay deliveries. (Use webhooks.verify() to validate inbound signatures.)

| Method | Endpoint | Description |
| --- | --- | --- |
| `create($body = null, array $opts = [])` | `POST /api/v1/webhooks` | Create a webhook endpoint (signing secret returned exactly once). |
| `list($query = null, array $opts = [])` | `GET /api/v1/webhooks` | List webhook endpoints. |
| `update($id, $body = null, array $opts = [])` | `PUT /api/v1/webhooks/:id` | Update a webhook endpoint. |
| `delete($id, $body = null, array $opts = [])` | `DELETE /api/v1/webhooks/:id` | Delete a webhook endpoint. |
| `test($id, $body = null, array $opts = [])` | `POST /api/v1/webhooks/:id/test` | Send a signed test event to the endpoint. |
| `rotateSecret($id, $body = null, array $opts = [])` | `POST /api/v1/webhooks/:id/rotate-secret` | Rotate the signing secret (new secret returned once). |
| `deliveries($id, $query = null, array $opts = [])` | `GET /api/v1/webhooks/:id/deliveries` | List delivery attempts for an endpoint. |
| `replay($id, $body = null, array $opts = [])` | `POST /api/v1/webhooks/deliveries/:id/replay` | Replay a past delivery. |
| `catalog($query = null, array $opts = [])` | `GET /api/v1/webhooks/catalog` | List every subscribable event type (no auth). |

<!-- END ENDPOINTS -->
