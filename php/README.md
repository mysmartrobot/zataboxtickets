# zatabox/zatabox PHP SDK

Official **PHP SDK** for the [Zatabox Tickets](https://zatabox.com) REST API the
white-label event-ticketing platform. A small, dependency-free client over
`https://api.zatabox.com/api/v1` that handles auth, sandbox routing, idempotency,
retries, pagination, binary downloads, file uploads and webhook verification.

- **Zero Composer dependencies** bundled `curl`, `json` and `hash` extensions.
- **Complete** every one of the **244 REST endpoints** is a method.
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
- [Binary downloads (PDF / CSV)](#binary-downloads-pdf--csv)
- [Live check-in stream (SSE)](#live-check-in-stream-sse)
- [File uploads](#file-uploads)
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

This SDK is **distributed via GitHub** — it is not published to Packagist. The package
lives in the `php/` directory of
[`mysmartrobot/zataboxtickets`](https://github.com/mysmartrobot/zataboxtickets).

**Option A — Composer path repository.** Add the repo as a submodule (or clone it),
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

**Option B — no Composer.** Clone the repo and require the bundled autoloader (it pulls
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
with your production account). A test key used against production or vice-versa —
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

`auth`, `users`, `savedSearches`, `dataExport`, `events`, `tickets`, `orders`,
`payments`, `checkin`, `scan`, `search`, `media`, `organizer`, `wallets`,
`scannerTokens`, `integrations`, `eventCustomization`, `growth`, `publicEvents`,
`site`, `community`, `track`, `webhooks`, `whiteLabel`, `support`, `util`.

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
$z->organizer->updateTicket($eventId, $ticketId, ['price' => 7500]);        // two params + body
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
foreach ($z->paginate([$z->organizer, 'events'], ['limit' => 50]) as $page) {
    foreach (($page['events'] ?? $page['items']) as $ev) {
        echo $ev['id'];
    }
}
```

`paginate(callable, query)` follows the cursor across both response shapes. To page
manually:

```php
$cursor = null;
do {
    $page = $z->organizer->events(['limit' => 50, 'cursor' => $cursor]);
    // ...
    $cursor = $page['pagination']['cursor'] ?? $page['nextCursor'] ?? null;
} while ($cursor);
```

## Binary downloads (PDF / CSV)

`tickets.pdf`, `orders.invoice`, `checkin.export`, `organizer.eventExport` and
`site.sitemap` return raw bytes:

```php
$pdf = $z->tickets->pdf($ticketId);   // ['data' => ..., 'contentType' => ..., 'filename' => ...]
file_put_contents($pdf['filename'] ?? 'ticket.pdf', $pdf['data']);
```

## Live check-in stream (SSE)

`$z->checkin->liveUrl($eventId)` returns the stream URL; consume it with an SSE client
(passing `Authorization: Bearer <key>`).

## File uploads

```php
$z->media->upload(file_get_contents('cover.jpg'), [
    'filename' => 'cover.jpg',
    'contentType' => 'image/jpeg',
    'fields' => ['alt' => 'Cover'],
]);
```

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

The SDK exposes **244 endpoints** across **26 namespaces**. Every method is listed below with its idiomatic signature, the underlying HTTP route, and what it does. Path parameters are positional; reads take an optional query map and writes take an optional body, both followed by a call-options bag.

### `$client->auth` Auth

Registration, password + passwordless sign-in, token refresh, 2FA and verification.

| Method | Endpoint | Description |
| --- | --- | --- |
| `register($body = null, array $opts = [])` | `POST /api/v1/auth/register` | Register a new user with email + password. |
| `login($body = null, array $opts = [])` | `POST /api/v1/auth/login` | Sign in with email + password; returns the JWT pair (or a 2FA challenge). |
| `loginVerify2fa($body = null, array $opts = [])` | `POST /api/v1/auth/2fa-verify` | Complete a login that returned a 2FA challenge. |
| `refresh($body = null, array $opts = [])` | `POST /api/v1/auth/refresh` | Exchange a refresh token for a fresh access/refresh pair. |
| `logout($body = null, array $opts = [])` | `POST /api/v1/auth/logout` | Revoke a refresh token. |
| `forgotPassword($body = null, array $opts = [])` | `POST /api/v1/auth/forgot-password` | Email a password-reset link. |
| `resetPassword($body = null, array $opts = [])` | `POST /api/v1/auth/reset-password` | Set a new password using a reset token. |
| `requestToken($body = null, array $opts = [])` | `POST /api/v1/auth/token/request` | Passwordless: email a 6-digit login code. |
| `exchangeToken($body = null, array $opts = [])` | `POST /api/v1/auth/token/exchange` | Passwordless: swap an emailed code for the JWT pair. |
| `verifyOtp($body = null, array $opts = [])` | `POST /api/v1/auth/verify-otp` | Verify a one-time passcode. |
| `verifyEmail($body = null, array $opts = [])` | `POST /api/v1/auth/verify-email` | Confirm an email address from a verification token. |
| `verifyPhone($body = null, array $opts = [])` | `POST /api/v1/auth/verify-phone` | Confirm a phone number from an SMS code. |
| `loginOauth($body = null, array $opts = [])` | `POST /api/v1/auth/login/oauth` | Sign in with a third-party OAuth identity token. |
| `enable2fa($body = null, array $opts = [])` | `POST /api/v1/auth/2fa/enable` | Begin enrolling TOTP two-factor auth. |
| `verify2fa($body = null, array $opts = [])` | `POST /api/v1/auth/2fa/verify` | Confirm a TOTP code to finish 2FA enrollment. |

### `$client->users` Users (Buyer)

The authenticated account: profile, wallet, tickets, orders, refunds, reports, messaging and notifications.

| Method | Endpoint | Description |
| --- | --- | --- |
| `me($query = null, array $opts = [])` | `GET /api/v1/users/me` | Current user profile. |
| `updateMe($body = null, array $opts = [])` | `PUT /api/v1/users/me` | Update the current user profile. |
| `orders($query = null, array $opts = [])` | `GET /api/v1/users/me/orders` | List the buyer's orders. |
| `tickets($query = null, array $opts = [])` | `GET /api/v1/users/me/tickets` | List the buyer's tickets across all organizers. |
| `deleteAccount($body = null, array $opts = [])` | `DELETE /api/v1/users/me` | Close the account. |
| `changePassword($body = null, array $opts = [])` | `POST /api/v1/users/me/password` | Change the account password. |
| `activity($query = null, array $opts = [])` | `GET /api/v1/users/me/activity` | Recent account activity. |
| `loginInfo($query = null, array $opts = [])` | `GET /api/v1/users/me/login-info` | Last-login metadata. |
| `twofaStatus($query = null, array $opts = [])` | `GET /api/v1/users/me/2fa/status` | Whether 2FA is enabled. |
| `twofaSetup($body = null, array $opts = [])` | `POST /api/v1/users/me/2fa/setup` | Start 2FA setup (returns the TOTP secret/QR). |
| `twofaEnable($body = null, array $opts = [])` | `POST /api/v1/users/me/2fa/enable` | Enable 2FA after verifying a code. |
| `twofaDisable($body = null, array $opts = [])` | `POST /api/v1/users/me/2fa/disable` | Disable 2FA. |
| `createRefund($body = null, array $opts = [])` | `POST /api/v1/users/me/refunds` | Submit a refund request for a ticket. |
| `refunds($query = null, array $opts = [])` | `GET /api/v1/users/me/refunds` | List the buyer's refund requests. |
| `withdrawRefund($id, $body = null, array $opts = [])` | `POST /api/v1/users/me/refunds/:id/withdraw` | Withdraw a pending refund request. |
| `createReport($body = null, array $opts = [])` | `POST /api/v1/users/me/reports` | File a report against an event or organizer. |
| `reports($query = null, array $opts = [])` | `GET /api/v1/users/me/reports` | List the buyer's filed reports. |
| `messages($query = null, array $opts = [])` | `GET /api/v1/users/me/messages` | Message threads with organizers. |
| `messageThread($threadId, $query = null, array $opts = [])` | `GET /api/v1/users/me/messages/:threadId` | A single message thread. |
| `sendTicketMessage($ticketId, $body = null, array $opts = [])` | `POST /api/v1/users/me/tickets/:ticketId/message` | Message the organizer of a ticket. |
| `notifications($query = null, array $opts = [])` | `GET /api/v1/users/me/notifications` | In-app notifications. |
| `notificationsUnreadCount($query = null, array $opts = [])` | `GET /api/v1/users/me/notifications/unread-count` | Count of unread notifications. |
| `markNotificationsRead($body = null, array $opts = [])` | `POST /api/v1/users/me/notifications/read` | Mark notifications as read. |
| `updateAvatar($body = null, array $opts = [])` | `PUT /api/v1/users/me/avatar` | Update the account avatar. |
| `updateNotificationSettings($body = null, array $opts = [])` | `PUT /api/v1/users/me/notifications/settings` | Update notification preferences. |

### `$client->savedSearches` Saved searches

The buyer's saved discovery searches.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list($query = null, array $opts = [])` | `GET /api/v1/users/me/saved-searches` | List saved searches. |
| `create($body = null, array $opts = [])` | `POST /api/v1/users/me/saved-searches` | Save a search. |
| `delete($id, $body = null, array $opts = [])` | `DELETE /api/v1/users/me/saved-searches/:id` | Delete a saved search. |

### `$client->dataExport` Data export

GDPR-style export of the account's data.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get($query = null, array $opts = [])` | `GET /api/v1/users/me/export` | Export all of the account's data. |

### `$client->events` Events (Public)

Public event discovery rails and per-event read panels.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list($query = null, array $opts = [])` | `GET /api/v1/events` | List / search public events (cursor-paginated). |
| `trending($query = null, array $opts = [])` | `GET /api/v1/events/trending` | Trending events. |
| `categories($query = null, array $opts = [])` | `GET /api/v1/events/categories` | Event categories with counts. |
| `nearby($query = null, array $opts = [])` | `GET /api/v1/events/nearby` | Events near a lat/lng or city. |
| `newThisWeek($query = null, array $opts = [])` | `GET /api/v1/events/new-this-week` | Recently published events. |
| `endingSoon($query = null, array $opts = [])` | `GET /api/v1/events/ending-soon` | Events with sales ending soon. |
| `free($query = null, array $opts = [])` | `GET /api/v1/events/free` | Free events. |
| `recommended($query = null, array $opts = [])` | `GET /api/v1/events/recommended` | Personalized recommendations. |
| `schedule($id, $query = null, array $opts = [])` | `GET /api/v1/events/:id/schedule` | An event's session schedule. |
| `organizer($id, $query = null, array $opts = [])` | `GET /api/v1/events/:id/organizer` | Public organizer profile for an event. |
| `faq($id, $query = null, array $opts = [])` | `GET /api/v1/events/:id/faq` | An event's FAQ. |
| `related($id, $query = null, array $opts = [])` | `GET /api/v1/events/:id/related` | Related events. |
| `expressInterest($id, $body = null, array $opts = [])` | `POST /api/v1/events/:id/interest` | Register interest in an event. |
| `issue($eventId, $body = null, array $opts = [])` | `POST /api/v1/events/:eventId/issue` | Externally issue a ticket for an event (integrator mode). |
| `get($slug, $query = null, array $opts = [])` | `GET /api/v1/events/:slug` | Get a public event by slug. |
| `tickets($id, $query = null, array $opts = [])` | `GET /api/v1/events/:id/tickets` | List an event's purchasable ticket types. |

### `$client->tickets` Tickets

Ticket QR/PDF, peer-to-peer transfers, promo validation and wallet passes.

| Method | Endpoint | Description |
| --- | --- | --- |
| `validatePromo($body = null, array $opts = [])` | `POST /api/v1/tickets/promo/validate` | Validate a promo code against a cart. |
| `qr($id, $query = null, array $opts = [])` | `GET /api/v1/tickets/:id/qr` | Current rotating QR payload for a ticket (JSON). |
| `pdf($id, $query = null, array $opts = [])` | `GET /api/v1/tickets/:id/pdf` | Ticket PDF (application/pdf bytes). |
| `transfer($id, $body = null, array $opts = [])` | `POST /api/v1/tickets/:id/transfer` | Initiate a peer-to-peer transfer; the recipient gets a claim link. |
| `revokeTransfer($transferId, $body = null, array $opts = [])` | `POST /api/v1/tickets/transfers/:transferId/revoke` | Revoke a still-pending transfer (initiator only). |
| `getTransfer($token, $query = null, array $opts = [])` | `GET /api/v1/tickets/transfers/claim/:token` | Inspect a pending transfer by claim token. |
| `claimTransfer($token, $body = null, array $opts = [])` | `POST /api/v1/tickets/transfers/claim/:token` | Claim a transfer; rewrites the ticket holder to the recipient. |
| `walletPass($id, $query = null, array $opts = [])` | `GET /api/v1/tickets/:id/wallet-pass` | Google/Apple wallet 'Save to Wallet' link (JSON). |
| `listByEvent($eventId, $query = null, array $opts = [])` | `GET /api/v1/tickets/:eventId` | List ticket types for an event (legacy /ticket-types mount). |

### `$client->orders` Orders

Carted checkout: create, pay, cancel, invoice.

| Method | Endpoint | Description |
| --- | --- | --- |
| `create($body = null, array $opts = [])` | `POST /api/v1/orders` | Create an order (reserves inventory). |
| `get($id, $query = null, array $opts = [])` | `GET /api/v1/orders/:id` | Order detail. |
| `cancel($id, $body = null, array $opts = [])` | `POST /api/v1/orders/:id/cancel` | Cancel an order and release its hold. |
| `pay($id, $body = null, array $opts = [])` | `POST /api/v1/orders/:id/pay` | Initiate payment for an order (nowpayments \| paystack \| flutterwave). |
| `invoice($id, $query = null, array $opts = [])` | `GET /api/v1/orders/:id/invoice` | Order receipt PDF (application/pdf bytes). |

### `$client->payments` Payments

Payment intent creation, verification, method discovery, and inbound provider webhooks.

| Method | Endpoint | Description |
| --- | --- | --- |
| `verify($body = null, array $opts = [])` | `POST /api/v1/payments/verify` | Confirm a charge with the provider and complete the order (idempotent). |
| `get($orderId, $query = null, array $opts = [])` | `GET /api/v1/payments/:orderId` | Payment status for an order. |
| `initiate($body = null, array $opts = [])` | `POST /api/v1/payments/initiate` | Create a payment intent. |
| `methods($query = null, array $opts = [])` | `GET /api/v1/payments/methods` | Available payment methods for a currency. |
| `cryptoCurrencies($query = null, array $opts = [])` | `GET /api/v1/payments/crypto/currencies` | Supported crypto currencies (NOWPayments). |
| `webhookNowpayments($body = null, array $opts = [])` | `POST /api/v1/payments/webhook/nowpayments` | Inbound NOWPayments webhook (provider callback; not for client use). |
| `webhookPaystack($body = null, array $opts = [])` | `POST /api/v1/payments/webhook/paystack` | Inbound Paystack webhook (provider callback; not for client use). |
| `webhookFlutterwave($body = null, array $opts = [])` | `POST /api/v1/payments/webhook/flutterwave` | Inbound Flutterwave webhook (provider callback; not for client use). |

### `$client->checkin` Check-in

Gate scanning, offline manifests, batch sync, live stats and CSV export.

| Method | Endpoint | Description |
| --- | --- | --- |
| `scan($body = null, array $opts = [])` | `POST /api/v1/checkin/scan` | Validate a QR / short-code ticket at a gate. |
| `batch($body = null, array $opts = [])` | `POST /api/v1/checkin/batch` | Flush a queue of scans captured offline. |
| `manual($id, $body = null, array $opts = [])` | `POST /api/v1/checkin/event/:id/manual` | Manually check in an attendee. |
| `manifest($id, $query = null, array $opts = [])` | `GET /api/v1/checkin/event/:id/manifest` | Offline manifest (ticket hashes + statuses); pass ?since for a delta. |
| `stats($id, $query = null, array $opts = [])` | `GET /api/v1/checkin/event/:id/stats` | Live check-in stats snapshot. |
| `registerDevice($body = null, array $opts = [])` | `POST /api/v1/checkin/device/register` | Register a scanning device. |
| `liveUrl($id, $query = null)` | `GET /api/v1/checkin/event/:id/live` | Server-Sent Events stream of live check-in stats. |
| `gate($id, $gate, $query = null, array $opts = [])` | `GET /api/v1/checkin/event/:id/gate/:gate` | Per-gate check-in stats. |
| `export($id, $query = null, array $opts = [])` | `GET /api/v1/checkin/event/:id/export` | Check-in log CSV (text/csv bytes). |

### `$client->scan` Scanner-token check-in

Passwordless gate scanning using a short-lived scanner token (the /scan kiosk surface).

| Method | Endpoint | Description |
| --- | --- | --- |
| `exchange($body = null, array $opts = [])` | `POST /api/v1/checkin-token/exchange` | Exchange a scanner token for a scoped session. |
| `session($query = null, array $opts = [])` | `GET /api/v1/checkin-token/me` | Current scanner session (event + gate context). |
| `scan($body = null, array $opts = [])` | `POST /api/v1/checkin-token/scan` | Validate a ticket with the scanner session. |
| `manifest($query = null, array $opts = [])` | `GET /api/v1/checkin-token/manifest` | Offline manifest for the scanner session. |
| `batch($body = null, array $opts = [])` | `POST /api/v1/checkin-token/batch` | Flush offline scans for the scanner session. |

### `$client->search` Search

Full-text + faceted event search.

| Method | Endpoint | Description |
| --- | --- | --- |
| `query($query = null, array $opts = [])` | `GET /api/v1/search` | Full-text + faceted search. |
| `suggest($query = null, array $opts = [])` | `GET /api/v1/search/suggest` | Type-ahead suggestions. |
| `trending($query = null, array $opts = [])` | `GET /api/v1/search/trending` | Trending search terms. |
| `popular($city, $query = null, array $opts = [])` | `GET /api/v1/search/popular/:city` | Popular searches in a city. |

### `$client->media` Media

Image/asset uploads and the stable /media/:id resolver.

| Method | Endpoint | Description |
| --- | --- | --- |
| `upload($body = null, array $opts = [])` | `POST /api/v1/media/upload` | Upload an image/asset (multipart/form-data). |

### `$client->organizer` Organizer

Authenticated organizer surface: organizations, members, events, ticket types, schedules, sections, promo codes, attendees, payouts, refunds, reports and messaging.

| Method | Endpoint | Description |
| --- | --- | --- |
| `setup($body = null, array $opts = [])` | `POST /api/v1/organizer/setup` | Bootstrap an organizer account + first organization. |
| `me($query = null, array $opts = [])` | `GET /api/v1/organizer/me` | Current organizer context (orgs + memberships). |
| `createOrganization($body = null, array $opts = [])` | `POST /api/v1/organizer/organizations` | Create an organization. |
| `getOrganization($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/organizations/:orgId` | Organization detail. |
| `updateOrganization($orgId, $body = null, array $opts = [])` | `PUT /api/v1/organizer/organizations/:orgId` | Update an organization. |
| `setOrganizationStatus($orgId, $body = null, array $opts = [])` | `PUT /api/v1/organizer/organizations/:orgId/status` | Change an organization's status. |
| `members($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/organizations/:orgId/members` | List organization members. |
| `invite($orgId, $body = null, array $opts = [])` | `POST /api/v1/organizer/organizations/:orgId/invites` | Invite a member. |
| `resendInvite($orgId, $memberId, $body = null, array $opts = [])` | `POST /api/v1/organizer/organizations/:orgId/invites/:memberId/resend` | Resend a member invite. |
| `removeMember($orgId, $memberId, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/organizations/:orgId/members/:memberId` | Remove a member. |
| `events($query = null, array $opts = [])` | `GET /api/v1/organizer/events` | List the organizer's events. |
| `createEvent($body = null, array $opts = [])` | `POST /api/v1/organizer/events` | Create a draft event. |
| `getEvent($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/events/:id` | Organizer event detail. |
| `updateEvent($id, $body = null, array $opts = [])` | `PUT /api/v1/organizer/events/:id` | Update an event. |
| `setEventStatus($id, $body = null, array $opts = [])` | `PUT /api/v1/organizer/events/:id/status` | Change an event's status. |
| `publishEvent($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/events/:id/publish` | Publish a draft event. |
| `unpublishEvent($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/events/:id/unpublish` | Unpublish a published event back to draft. |
| `deleteEvent($id, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/events/:id` | Cancel/delete an event. |
| `tickets($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/events/:id/tickets` | List an event's ticket types. |
| `createTicket($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/events/:id/tickets` | Create a ticket type. |
| `updateTicket($id, $tid, $body = null, array $opts = [])` | `PUT /api/v1/organizer/events/:id/tickets/:tid` | Update a ticket type. |
| `deleteTicket($id, $tid, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/events/:id/tickets/:tid` | Delete a ticket type. |
| `schedule($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/events/:id/schedule` | List schedule sessions. |
| `createSchedule($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/events/:id/schedule` | Add a schedule session. |
| `updateSchedule($id, $sid, $body = null, array $opts = [])` | `PUT /api/v1/organizer/events/:id/schedule/:sid` | Update a schedule session. |
| `deleteSchedule($id, $sid, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/events/:id/schedule/:sid` | Delete a schedule session. |
| `sections($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/events/:id/sections` | List seating/venue sections. |
| `createSection($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/events/:id/sections` | Add a section. |
| `updateSection($id, $sid, $body = null, array $opts = [])` | `PUT /api/v1/organizer/events/:id/sections/:sid` | Update a section. |
| `deleteSection($id, $sid, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/events/:id/sections/:sid` | Delete a section. |
| `promoCodes($query = null, array $opts = [])` | `GET /api/v1/organizer/promo-codes` | List promo codes. |
| `createPromoCode($body = null, array $opts = [])` | `POST /api/v1/organizer/promo-codes` | Create a promo code. |
| `updatePromoCode($id, $body = null, array $opts = [])` | `PUT /api/v1/organizer/promo-codes/:id` | Update a promo code. |
| `deletePromoCode($id, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/promo-codes/:id` | Delete a promo code. |
| `eventAnalytics($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/events/:id/analytics` | Sales/analytics for an event. |
| `eventAttendees($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/events/:id/attendees` | Attendee CRM list for an event. |
| `eventExport($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/events/:id/export` | Attendee export CSV (text/csv bytes). |
| `payouts($query = null, array $opts = [])` | `GET /api/v1/organizer/payouts` | List payouts. |
| `payout($id, $query = null, array $opts = [])` | `GET /api/v1/organizer/payouts/:id` | Payout detail. |
| `requestPayout($body = null, array $opts = [])` | `POST /api/v1/organizer/payouts/request` | Request a payout. |
| `updatePayoutSettings($body = null, array $opts = [])` | `PUT /api/v1/organizer/payout-settings` | Update payout settings. |
| `refunds($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/orgs/:orgId/refunds` | List refund requests for an org. |
| `decideRefund($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/refunds/:id/decide` | Approve or deny a refund request. |
| `reports($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/orgs/:orgId/reports` | List reports filed against an org (read-only). |
| `resolveReport($id, $body = null, array $opts = [])` | `POST /api/v1/organizer/reports/:id/resolve` | Resolve a report (admins only; organizers receive 403). |
| `messages($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/orgs/:orgId/messages` | Org-wide buyer message threads. |
| `overview($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/orgs/:orgId/overview` | Org dashboard overview. |
| `referral($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/orgs/:orgId/referral` | Referral program summary + commissions. |
| `notifications($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/orgs/:orgId/notifications` | Org notifications. |
| `fundWallet($orgId, $body = null, array $opts = [])` | `POST /api/v1/organizer/wallets/org/:orgId/fund` | Top up the org wallet (returns a payment intent). |
| `fundWalletStatus($orgId, $orderId, $query = null, array $opts = [])` | `GET /api/v1/organizer/wallets/org/:orgId/fund/:orderId` | Poll a wallet top-up payment. |
| `payoutDetails($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/orgs/:orgId/payout-details` | Stored payout destinations. |
| `updatePayoutDetails($orgId, $currency, $body = null, array $opts = [])` | `PUT /api/v1/organizer/orgs/:orgId/payout-details/:currency` | Set payout details for a currency. |
| `ticketMessage($ticketId, $body = null, array $opts = [])` | `POST /api/v1/organizer/tickets/:ticketId/message` | Reply to a buyer on a ticket thread. |
| `messageThread($threadId, $query = null, array $opts = [])` | `GET /api/v1/organizer/messages/:threadId` | A single org message thread. |

### `$client->wallets` Wallets

Organization wallet balances and ledger.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list($query = null, array $opts = [])` | `GET /api/v1/organizer/wallets` | List wallets the caller can see. |
| `get($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/wallets/org/:orgId` | An org's wallet balances (per currency). |
| `bootstrap($orgId, $body = null, array $opts = [])` | `POST /api/v1/organizer/wallets/org/:orgId/bootstrap` | Provision an org's wallet. |
| `transactions($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/wallets/org/:orgId/transactions` | Wallet ledger transactions. |

### `$client->scannerTokens` Scanner tokens

Manage short-lived gate-scanner tokens for staff devices.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/scanner-tokens/org/:orgId` | List scanner tokens. |
| `create($orgId, $body = null, array $opts = [])` | `POST /api/v1/organizer/scanner-tokens/org/:orgId` | Mint a scanner token. |
| `update($orgId, $tokenId, $body = null, array $opts = [])` | `PUT /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId` | Update a scanner token. |
| `delete($orgId, $tokenId, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId` | Revoke a scanner token. |
| `reissue($orgId, $tokenId, $body = null, array $opts = [])` | `POST /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId/reissue` | Reissue a scanner token's secret. |
| `metrics($orgId, $tokenId, $query = null, array $opts = [])` | `GET /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId/metrics` | Usage metrics for a scanner token. |

### `$client->integrations` Integrations

API keys, MCP tokens, and integration usage metrics.

| Method | Endpoint | Description |
| --- | --- | --- |
| `metrics($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/integrations/org/:orgId/metrics` | Integration usage metrics. |
| `apiCalls($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/integrations/org/:orgId/api-calls` | Recent API call log. |
| `mcpCalls($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/integrations/org/:orgId/mcp-calls` | Recent MCP tool-call log. |
| `listApiKeys($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/integrations/org/:orgId/api-keys` | List API keys. |
| `createApiKey($orgId, $body = null, array $opts = [])` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys` | Mint an API key (plaintext returned once). |
| `updateApiKey($orgId, $keyId, $body = null, array $opts = [])` | `PUT /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Update an API key (scopes, status, allowlist). |
| `rotateApiKey($orgId, $keyId, $body = null, array $opts = [])` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId/rotate` | Rotate an API key's secret. |
| `deleteApiKey($orgId, $keyId, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Revoke an API key. |
| `listMcpTokens($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/integrations/org/:orgId/mcp-tokens` | List MCP tokens. |
| `createMcpToken($orgId, $body = null, array $opts = [])` | `POST /api/v1/organizer/integrations/org/:orgId/mcp-tokens` | Mint an MCP token. |
| `updateMcpToken($orgId, $tokenId, $body = null, array $opts = [])` | `PUT /api/v1/organizer/integrations/org/:orgId/mcp-tokens/:tokenId` | Update an MCP token. |
| `deleteMcpToken($orgId, $tokenId, $body = null, array $opts = [])` | `DELETE /api/v1/organizer/integrations/org/:orgId/mcp-tokens/:tokenId` | Revoke an MCP token. |

### `$client->eventCustomization` Event customization

Per-event white-label theming.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get($eventId, $query = null, array $opts = [])` | `GET /api/v1/organizer/event-customization/:eventId` | Get an event's customization. |
| `update($eventId, $body = null, array $opts = [])` | `PUT /api/v1/organizer/event-customization/:eventId` | Update an event's customization. |

### `$client->growth` Growth

Comp tickets, CSV import, broadcasts and attendee tags.

| Method | Endpoint | Description |
| --- | --- | --- |
| `mintComps($eventId, $body = null, array $opts = [])` | `POST /api/v1/organizer/growth/events/:eventId/comps` | Issue complimentary tickets. |
| `importCompsCsv($eventId, $body = null, array $opts = [])` | `POST /api/v1/organizer/growth/events/:eventId/comps/import-csv` | Bulk-issue comps from CSV. |
| `listComps($eventId, $query = null, array $opts = [])` | `GET /api/v1/organizer/growth/events/:eventId/comps` | List issued comps. |
| `resendComp($eventId, $ticketId, $body = null, array $opts = [])` | `POST /api/v1/organizer/growth/events/:eventId/comps/:ticketId/resend` | Resend a comp ticket email. |
| `broadcastEvent($eventId, $body = null, array $opts = [])` | `POST /api/v1/organizer/growth/events/:eventId/broadcast` | Broadcast to an event's attendees. |
| `broadcastOrg($orgId, $body = null, array $opts = [])` | `POST /api/v1/organizer/growth/orgs/:orgId/broadcast` | Broadcast to an org's followers/subscribers. |
| `listBroadcasts($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/growth/orgs/:orgId/broadcasts` | List sent broadcasts. |
| `addTags($body = null, array $opts = [])` | `POST /api/v1/organizer/growth/tags` | Tag attendees (body: orgId, ticketIds, tag). |
| `removeTag($body = null, array $opts = [])` | `DELETE /api/v1/organizer/growth/tags` | Remove a tag (body: orgId, ticketId, tag). |
| `listTags($orgId, $query = null, array $opts = [])` | `GET /api/v1/organizer/growth/orgs/:orgId/tags` | List tags for an org. |
| `tagAttendees($orgId, $tag, $query = null, array $opts = [])` | `GET /api/v1/organizer/growth/orgs/:orgId/tags/:tag/attendees` | List attendees with a given tag. |

### `$client->publicEvents` Public events (vanity)

Public org/event read endpoints used by hosted pages and white-label sites.

| Method | Endpoint | Description |
| --- | --- | --- |
| `getBySlug($slug, $query = null, array $opts = [])` | `GET /api/v1/public/events/:slug` | Public event by slug. |
| `orgEvents($orgId, $query = null, array $opts = [])` | `GET /api/v1/public/events/orgs/by/:orgId` | An organization's public events. |
| `getByOrgEvent($orgId, $eventId, $query = null, array $opts = [])` | `GET /api/v1/public/events/by/:orgId/:eventId` | Public event by org + event id. |
| `getById($eventId, $query = null, array $opts = [])` | `GET /api/v1/public/events/by-id/:eventId` | Public event by id. |
| `preview($eventId, $query = null, array $opts = [])` | `GET /api/v1/public/events/preview/:eventId` | Draft event preview (token-gated). |

### `$client->site` Public site

Sitemap, newsletter signup and platform status.

| Method | Endpoint | Description |
| --- | --- | --- |
| `sitemap($query = null, array $opts = [])` | `GET /api/v1/public/sitemap.xml` | Sitemap XML (application/xml bytes). |
| `newsletterStart($body = null, array $opts = [])` | `POST /api/v1/public/newsletter/start` | Begin newsletter double-opt-in. |
| `newsletterConfirm($body = null, array $opts = [])` | `POST /api/v1/public/newsletter` | Confirm a newsletter subscription. |
| `status($query = null, array $opts = [])` | `GET /api/v1/public/status` | Platform status + incidents. |

### `$client->community` Community

Verified-attendee reviews, organizer follows and event waitlists.

| Method | Endpoint | Description |
| --- | --- | --- |
| `submitReview($body = null, array $opts = [])` | `POST /api/v1/community/reviews` | Submit a verified-attendee review (ticketCode + email prove ownership). |
| `orgReviews($orgId, $query = null, array $opts = [])` | `GET /api/v1/community/orgs/:orgId/reviews` | Published reviews for an org. |
| `eventReviews($eventId, $query = null, array $opts = [])` | `GET /api/v1/community/events/:eventId/reviews` | Published reviews for an event. |
| `follow($orgId, $body = null, array $opts = [])` | `POST /api/v1/community/orgs/:orgId/follow` | Follow an organizer. |
| `unfollow($token, $query = null, array $opts = [])` | `GET /api/v1/community/unfollow/:token` | Unsubscribe via emailed token. |
| `joinWaitlist($eventId, $body = null, array $opts = [])` | `POST /api/v1/community/events/:eventId/waitlist` | Join an event waitlist. |
| `acceptWaitlist($id, $token, $query = null, array $opts = [])` | `GET /api/v1/community/waitlist/accept/:id/:token` | Accept a waitlist offer via emailed token. |
| `manageReviews($orgId, $query = null, array $opts = [])` | `GET /api/v1/community/orgs/:orgId/reviews/manage` | Organizer view of all reviews (incl. pending). |
| `replyReview($id, $body = null, array $opts = [])` | `POST /api/v1/community/reviews/:id/reply` | Organizer reply to a review. |
| `setReviewStatus($id, $body = null, array $opts = [])` | `POST /api/v1/community/reviews/:id/status` | Publish/hide a review. |
| `followers($orgId, $query = null, array $opts = [])` | `GET /api/v1/community/orgs/:orgId/followers` | List an org's followers. |
| `removeFollower($orgId, $followerId, $body = null, array $opts = [])` | `DELETE /api/v1/community/orgs/:orgId/followers/:followerId` | Remove a follower. |
| `eventWaitlist($eventId, $query = null, array $opts = [])` | `GET /api/v1/community/events/:eventId/waitlist` | List an event's waitlist. |
| `offerWaitlist($eventId, $body = null, array $opts = [])` | `POST /api/v1/community/events/:eventId/waitlist/offer` | Offer spots to waitlisted attendees. |

### `$client->track` Tracking

Lightweight page-view tracking.

| Method | Endpoint | Description |
| --- | --- | --- |
| `view($body = null, array $opts = [])` | `POST /api/v1/track/view` | Record a page view. |

### `$client->webhooks` Webhooks

Register webhook endpoints and inspect/replay deliveries. (Use client.webhooks.verify() to validate inbound signatures.)

| Method | Endpoint | Description |
| --- | --- | --- |
| `catalog($query = null, array $opts = [])` | `GET /api/v1/webhooks/catalog` | List subscribable event types. |
| `list($query = null, array $opts = [])` | `GET /api/v1/webhooks` | List registered webhook endpoints. |
| `create($body = null, array $opts = [])` | `POST /api/v1/webhooks` | Register a webhook endpoint. |
| `update($id, $body = null, array $opts = [])` | `PUT /api/v1/webhooks/:id` | Update a webhook endpoint. |
| `rotateSecret($id, $body = null, array $opts = [])` | `POST /api/v1/webhooks/:id/rotate-secret` | Rotate a webhook signing secret. |
| `delete($id, $body = null, array $opts = [])` | `DELETE /api/v1/webhooks/:id` | Delete a webhook endpoint. |
| `deliveries($id, $query = null, array $opts = [])` | `GET /api/v1/webhooks/:id/deliveries` | List delivery attempts for an endpoint. |
| `test($id, $body = null, array $opts = [])` | `POST /api/v1/webhooks/:id/test` | Send a test event to an endpoint. |
| `replay($id, $body = null, array $opts = [])` | `POST /api/v1/webhooks/deliveries/:id/replay` | Replay a past delivery. |

### `$client->whiteLabel` White-label

Self-contained white-label surface: events, ticket types, customization, orders, tickets and check-in under one integration.

| Method | Endpoint | Description |
| --- | --- | --- |
| `me($query = null, array $opts = [])` | `GET /api/v1/white-label/me` | The integration's white-label context. |
| `events($query = null, array $opts = [])` | `GET /api/v1/white-label/events` | List white-label events. |
| `createEvent($body = null, array $opts = [])` | `POST /api/v1/white-label/events` | Create a white-label event. |
| `getEvent($id, $query = null, array $opts = [])` | `GET /api/v1/white-label/events/:id` | White-label event detail. |
| `updateEvent($id, $body = null, array $opts = [])` | `PUT /api/v1/white-label/events/:id` | Update a white-label event. |
| `deleteEvent($id, $body = null, array $opts = [])` | `DELETE /api/v1/white-label/events/:id` | Delete a white-label event. |
| `ticketTypes($eventId, $query = null, array $opts = [])` | `GET /api/v1/white-label/events/:eventId/ticket-types` | List ticket types. |
| `createTicketType($eventId, $body = null, array $opts = [])` | `POST /api/v1/white-label/events/:eventId/ticket-types` | Create a ticket type. |
| `getCustomization($eventId, $query = null, array $opts = [])` | `GET /api/v1/white-label/events/:eventId/customization` | Get event customization. |
| `updateCustomization($eventId, $body = null, array $opts = [])` | `PUT /api/v1/white-label/events/:eventId/customization` | Update event customization. |
| `orders($query = null, array $opts = [])` | `GET /api/v1/white-label/orders` | List white-label orders. |
| `tickets($query = null, array $opts = [])` | `GET /api/v1/white-label/tickets` | List white-label tickets. |
| `checkinScan($body = null, array $opts = [])` | `POST /api/v1/white-label/checkin/scan` | Validate a ticket at a white-label gate. |
| `checkinStats($eventId, $query = null, array $opts = [])` | `GET /api/v1/white-label/checkin/stats/:eventId` | White-label check-in stats. |
| `wallets($query = null, array $opts = [])` | `GET /api/v1/white-label/wallets` | White-label wallet balances. |

### `$client->support` Support

Support tickets and threaded messages.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list($query = null, array $opts = [])` | `GET /api/v1/support` | List support tickets. |
| `create($body = null, array $opts = [])` | `POST /api/v1/support` | Open a support ticket. |
| `get($id, $query = null, array $opts = [])` | `GET /api/v1/support/:id` | Support ticket detail. |
| `sendMessage($id, $body = null, array $opts = [])` | `POST /api/v1/support/:id/messages` | Reply on a support ticket. |

### `$client->util` Utilities

Helper endpoints.

| Method | Endpoint | Description |
| --- | --- | --- |
| `resolveCoords($body = null, array $opts = [])` | `POST /api/v1/util/resolve-coords` | Resolve an address to coordinates. |

<!-- END ENDPOINTS -->
