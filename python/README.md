# zatabox Python SDK

Official **Python SDK** for the [Zatabox Tickets](https://zatabox.com) REST API the
white-label event-ticketing platform. A small, dependency-free client over
`https://api.zatabox.com/api/v1` that handles auth, sandbox routing, idempotency,
retries, pagination, binary downloads, file uploads and webhook verification.

- **Zero dependencies** standard library only (`urllib`, `hmac`, `hashlib`).
- **Complete** every one of the **244 REST endpoints** is a method.
- **Generated, never drifts** emitted from the canonical
  [`endpoints.json`](../spec/endpoints.json) spec.
- **Python 3.7+**.

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
- [Thread safety](#thread-safety)
- [Troubleshooting & FAQ](#troubleshooting--faq)
- [Full endpoint reference](#full-endpoint-reference)
- [Versioning & support](#versioning--support)
- [License](#license)

---

## Requirements

- **Python 3.7 or newer**. No third-party packages.
- A Zatabox **API key** (`vt_live_…` / `vt_test_…`), a **portal JWT**, or an **MCP
  token**. Mint API keys in the organizer portal → Integrations, or the
  [sandbox console](https://tester.zatabox.com).

## Installation

This SDK is **distributed via GitHub** — it is not published to PyPI. The package lives
in the `python/` directory of
[`mysmartrobot/zataboxtickets`](https://github.com/mysmartrobot/zataboxtickets), and pip
installs it straight from there with `#subdirectory=python`:

```bash
pip install "git+https://github.com/mysmartrobot/zataboxtickets.git#subdirectory=python"
```

Pin to a tag, branch or commit for reproducible builds:

```bash
pip install "git+https://github.com/mysmartrobot/zataboxtickets.git@v0.3.0#subdirectory=python"
```

Or add it to `requirements.txt` / `pyproject.toml`:

```text
# requirements.txt
zatabox @ git+https://github.com/mysmartrobot/zataboxtickets.git#subdirectory=python
```

The SDK has **zero dependencies**. Then import it:

```python
from zatabox import Client, ZataboxError
```

## Quick start

```python
import os
from zatabox import Client, ZataboxError

# A vt_test_ key auto-routes to the sandbox; a vt_live_ key to production.
z = Client(api_key=os.environ["ZATABOX_API_KEY"])

try:
    event = z.organizer.create_event({
        "title": "Warehouse Sessions 004",
        "category": "music",
        "startDate": "2026-08-22T20:00:00Z",
        "endDate": "2026-08-23T02:00:00Z",
        "timezone": "Africa/Lagos",
        "venueType": "physical",
        "venueCity": "Lagos",
        "capacity": 450,
    })
    z.organizer.create_ticket(event["id"], {
        "name": "General Admission", "type": "general", "price": 5000,
        "currency": "NGN", "quantityTotal": 450,
        "saleStart": "2026-07-01T00:00:00Z", "saleEnd": "2026-08-22T20:00:00Z",
    })
    z.organizer.publish_event(event["id"])
    print("published:", z.events.get(event["slug"])["status"])
except ZataboxError as e:
    print(e.code, e.message, e.request_id)
```

## Authentication

The SDK forwards one `Authorization: Bearer <token>` header. Three ways to authenticate:

```python
Client(api_key="vt_live_...")    # scoped API key (prefix selects environment)
Client(api_key="vt_test_...")    # sandbox (auto-routed)
Client(bearer_token="eyJ...")    # portal JWT or vt_mcp_ token
Client(api_key="vt_test_...", base_url="http://localhost:4100")  # explicit override
```

### Passwordless buyer login

```python
anon = Client(bearer_token="unused", base_url="https://api.zatabox.com")
anon.auth.request_token({"email": "fan@example.com"})            # emails a 6-digit code
session = anon.auth.exchange_token({"email": "fan@example.com", "code": "123456"})

buyer = Client(bearer_token=session["accessToken"])
tickets = buyer.users.tickets()
```

### Refreshing & swapping tokens

```python
nxt = buyer.auth.refresh({"refreshToken": session["refreshToken"]})
buyer.set_bearer_token(nxt["accessToken"])
```

### API-key scopes

Keys can be minted with least-privilege scopes: `events:read`, `events:write`,
`tickets:read`, `tickets:write`, `orders:read`, `orders:write`, `attendees:read`,
`attendees:write`, `checkin:write`, `payouts:read`, `payouts:write`, `webhooks:manage`,
`analytics:read`, `*`. A call beyond a key's scopes returns `403 INSUFFICIENT_SCOPE`.

## Sandbox / test mode

`vt_test_` keys auto-route to the Zatabox **sandbox** at `https://sandbox.zatabox.com`
a full mirror of the API with no real charges, emails or SMS.

```python
Client(api_key="vt_test_...")   # → sandbox.zatabox.com
Client(api_key="vt_live_...")   # → api.zatabox.com
Client(api_key="vt_test_...", base_url="http://localhost:4100")  # self-hosted
```

Mint/rotate `vt_test_` keys, watch **live request logs**, see usage and browse the
endpoint catalog in the **sandbox console at https://tester.zatabox.com** (sign in
with your production account). A test key used against production or vice-versa —
returns `403 WRONG_ENV`.

## Client configuration

```python
Client(
    api_key="vt_live_...",   # or bearer_token=...
    base_url=None,           # override the auto-resolved host
    timeout=30.0,            # per-request timeout (seconds)
    max_retries=2,           # retries for 5xx / network / timeout
    user_agent=None,         # defaults to zatabox-python/<version>
)
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `api_key` | `str` | | `vt_live_…` / `vt_test_…`; test keys auto-route to the sandbox. |
| `bearer_token` | `str` | | Portal JWT / `vt_mcp_…`. One of `api_key`/`bearer_token` required. |
| `base_url` | `str` | resolved from key | Explicit API origin; wins over prefix routing. |
| `timeout` | `float` | `30.0` | Per-request timeout in seconds. |
| `max_retries` | `int` | `2` | Retries for `5xx`/network/timeout (never `4xx`). |
| `user_agent` | `str` | `zatabox-python/<version>` | Overrides the `User-Agent` header. |

## How methods map to endpoints

`client.<namespace>.<method>(...)`. Namespaces are snake_case:

`auth`, `users`, `saved_searches`, `data_export`, `events`, `tickets`, `orders`,
`payments`, `checkin`, `scan`, `search`, `media`, `organizer`, `wallets`,
`scanner_tokens`, `integrations`, `event_customization`, `growth`, `public_events`,
`site`, `community`, `track`, `webhooks`, `white_label`, `support`, `util`.

Argument order:

```
method(path_param1, path_param2, …, payload=None, **opts)
```

- **Path params** first (URL-encoded for you).
- **Reads** take an optional `query` dict; **writes** take an optional `body` dict.
- **`**opts`** carries `idempotency_key=`, `headers=`, and (for writes) an extra
  `query=`.

```python
z.events.list({"q": "jazz", "city": "Lagos", "limit": 20})           # query
z.events.get("warehouse-sessions-004")                               # path param
z.organizer.update_ticket(event_id, ticket_id, {"price": 7500})      # two params + body
z.orders.create(cart, idempotency_key=my_uuid)                       # body + opts
```

## Responses

The API wraps success as `{ "success", "data", "meta" }`; the SDK **returns `data`
directly**. Pagination cursors and `request_id` live inside that document.

```python
page = z.events.list({"limit": 20})   # e.g. {"items": [...], "pagination": {...}}
```

## Error handling

Any non-2xx (and network/timeout/webhook failures) raises **`ZataboxError`**:

```python
from zatabox import ZataboxError

try:
    z.orders.create({"items": items})
except ZataboxError as e:
    e.code         # 'TICKET_SOLD_OUT'
    e.status       # 409 (0 for network errors)
    e.message      # human-readable
    e.request_id   # 'req_01J9...'
    e.details      # {'ticketTypeId': 'tkt_8f2k'}
```

### Common error codes

| `code` | `status` | Meaning |
| --- | --- | --- |
| `VALIDATION_ERROR` | 400 | Body failed validation (`details['issues']`). |
| `UNAUTHORIZED` / `INVALID_TOKEN` | 401 | Missing/expired credential. |
| `API_KEY_REQUIRED` / `API_KEY_INVALID` | 401 | Bad/missing API key. |
| `WRONG_ENV` | 403 | Test key on production or vice-versa. |
| `INSUFFICIENT_SCOPE` | 403 | Key lacks the route's scope. |
| `NOT_FOUND` | 404 | No such resource. |
| `CONFLICT` / `IDEMPOTENCY_KEY_REUSED` | 409 | Unique/idempotency clash. |
| `TICKET_SOLD_OUT` | 409 | Inventory exhausted. |
| `RATE_LIMITED` | 429 | Throttled; see `details['retryAfter']`. |
| `INTERNAL_ERROR` | 500 | Server error (auto-retried). |
| `NETWORK_ERROR` | 0 | Connection/timeout after retries (SDK-side). |
| `MISSING_SIGNATURE` / `INVALID_SIGNATURE` / `SIGNATURE_EXPIRED` | | webhook verify failures. |

## Idempotency

Every write auto-sends an `Idempotency-Key` (fresh UUIDv4). The server caches the
result for 24h replaying the same key + body returns the original; the same key with
a different body returns `409 IDEMPOTENCY_KEY_REUSED`. Pass your own to make a retry
safe:

```python
import uuid
key = str(uuid.uuid4())
z.orders.create(cart, idempotency_key=key)
z.orders.create(cart, idempotency_key=key)   # safe retry no double charge
```

## Retries, timeouts & networking

- **Timeouts** each request is bounded by `timeout` (seconds); a timeout is a
  retryable network error.
- **Retries** `5xx`/network/timeout retried up to `max_retries` with exponential
  backoff; `4xx` never retried.
- **Rate limits** `429` raises `ZataboxError(code="RATE_LIMITED")` with
  `details["retryAfter"]` when present.

## Pagination

```python
for page in z.paginate(z.organizer.events, {"limit": 50}):
    for ev in page.get("events") or page.get("items"):
        print(ev["id"])
```

`paginate(list_method, query)` follows the cursor across both response shapes. To page
manually:

```python
cursor = None
while True:
    page = z.organizer.events({"limit": 50, "cursor": cursor})
    ...
    cursor = (page.get("pagination") or {}).get("cursor") or page.get("nextCursor")
    if not cursor:
        break
```

## Binary downloads (PDF / CSV)

`tickets.pdf`, `orders.invoice`, `checkin.export`, `organizer.event_export` and
`site.sitemap` return raw bytes:

```python
pdf = z.tickets.pdf(ticket_id)   # {"data": b"...", "content_type": ..., "filename": ...}
with open(pdf["filename"] or "ticket.pdf", "wb") as f:
    f.write(pdf["data"])
```

## Live check-in stream (SSE)

`checkin.live_url(event_id)` returns the stream URL; consume it with your preferred SSE
client (passing `Authorization: Bearer <key>`):

```python
url = z.checkin.live_url(event_id)
# e.g. with `sseclient`/`httpx` each `stats` event carries a JSON snapshot.
```

## File uploads

```python
with open("cover.jpg", "rb") as f:
    asset = z.media.upload(f.read(), filename="cover.jpg", content_type="image/jpeg",
                           fields={"alt": "Cover"})
```

## Verifying inbound webhooks

Signature header: `X-Zatabox-Signature: t=<unix>,v1=<hex-hmac-sha256>`; the signed
payload is `<t>.<raw_body>`, HMAC-SHA256 with your endpoint secret (constant-time, 5-min
tolerance). **Verify the raw bytes, not a re-serialized object.**

```python
# Flask example
from flask import request, abort

@app.post("/zatabox/webhooks")
def hook():
    try:
        event = z.webhooks.verify(
            request.get_data(as_text=True),
            request.headers.get("X-Zatabox-Signature"),
            os.environ["ZATABOX_WEBHOOK_SECRET"],
        )
    except ZataboxError:
        abort(400)
    if event["type"] == "order.paid":
        ...  # fulfil
    return "", 200
```

## End-to-end recipes

```python
# Sell a ticket (guest checkout)
order = z.orders.create({"items": [{"ticketTypeId": tt, "quantity": 2}],
                         "guestEmail": "fan@example.com"})
intent = z.orders.pay(order["id"], {"provider": "paystack"})
paid = z.payments.verify({"orderId": order["id"]})

# Check in at the gate
res = z.checkin.scan({"qrData": qr, "gateName": "Main", "deviceId": device})
stats = z.checkin.stats(event_id)
```

## Thread safety

A `Client` holds only its credential and configuration and is safe to share across
threads (each call opens its own `urllib` connection). Create one per credential and
reuse it.

## Troubleshooting & FAQ

- **`ValueError: pass either api_key or bearer_token`** provide a credential.
- **`403 WRONG_ENV`** match the key prefix to the environment.
- **`409 IDEMPOTENCY_KEY_REUSED`** reused a key with a different body; use a new one.
- **Webhook `INVALID_SIGNATURE`** verify the raw request body, not parsed JSON.
- **A list looks short** iterate with `z.paginate(...)`.

## Versioning & support

SemVer; version is `zatabox.__version__`. API base `https://api.zatabox.com/api/v1` ·
Docs <https://zatabox.com/docs> · developers@zatabox.com.

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
| `register(body=None, **opts)` | `POST /api/v1/auth/register` | Register a new user with email + password. |
| `login(body=None, **opts)` | `POST /api/v1/auth/login` | Sign in with email + password; returns the JWT pair (or a 2FA challenge). |
| `login_verify2fa(body=None, **opts)` | `POST /api/v1/auth/2fa-verify` | Complete a login that returned a 2FA challenge. |
| `refresh(body=None, **opts)` | `POST /api/v1/auth/refresh` | Exchange a refresh token for a fresh access/refresh pair. |
| `logout(body=None, **opts)` | `POST /api/v1/auth/logout` | Revoke a refresh token. |
| `forgot_password(body=None, **opts)` | `POST /api/v1/auth/forgot-password` | Email a password-reset link. |
| `reset_password(body=None, **opts)` | `POST /api/v1/auth/reset-password` | Set a new password using a reset token. |
| `request_token(body=None, **opts)` | `POST /api/v1/auth/token/request` | Passwordless: email a 6-digit login code. |
| `exchange_token(body=None, **opts)` | `POST /api/v1/auth/token/exchange` | Passwordless: swap an emailed code for the JWT pair. |
| `verify_otp(body=None, **opts)` | `POST /api/v1/auth/verify-otp` | Verify a one-time passcode. |
| `verify_email(body=None, **opts)` | `POST /api/v1/auth/verify-email` | Confirm an email address from a verification token. |
| `verify_phone(body=None, **opts)` | `POST /api/v1/auth/verify-phone` | Confirm a phone number from an SMS code. |
| `login_oauth(body=None, **opts)` | `POST /api/v1/auth/login/oauth` | Sign in with a third-party OAuth identity token. |
| `enable2fa(body=None, **opts)` | `POST /api/v1/auth/2fa/enable` | Begin enrolling TOTP two-factor auth. |
| `verify2fa(body=None, **opts)` | `POST /api/v1/auth/2fa/verify` | Confirm a TOTP code to finish 2FA enrollment. |

### `client.users` Users (Buyer)

The authenticated account: profile, wallet, tickets, orders, refunds, reports, messaging and notifications.

| Method | Endpoint | Description |
| --- | --- | --- |
| `me(query=None, **opts)` | `GET /api/v1/users/me` | Current user profile. |
| `update_me(body=None, **opts)` | `PUT /api/v1/users/me` | Update the current user profile. |
| `orders(query=None, **opts)` | `GET /api/v1/users/me/orders` | List the buyer's orders. |
| `tickets(query=None, **opts)` | `GET /api/v1/users/me/tickets` | List the buyer's tickets across all organizers. |
| `delete_account(body=None, **opts)` | `DELETE /api/v1/users/me` | Close the account. |
| `change_password(body=None, **opts)` | `POST /api/v1/users/me/password` | Change the account password. |
| `activity(query=None, **opts)` | `GET /api/v1/users/me/activity` | Recent account activity. |
| `login_info(query=None, **opts)` | `GET /api/v1/users/me/login-info` | Last-login metadata. |
| `twofa_status(query=None, **opts)` | `GET /api/v1/users/me/2fa/status` | Whether 2FA is enabled. |
| `twofa_setup(body=None, **opts)` | `POST /api/v1/users/me/2fa/setup` | Start 2FA setup (returns the TOTP secret/QR). |
| `twofa_enable(body=None, **opts)` | `POST /api/v1/users/me/2fa/enable` | Enable 2FA after verifying a code. |
| `twofa_disable(body=None, **opts)` | `POST /api/v1/users/me/2fa/disable` | Disable 2FA. |
| `create_refund(body=None, **opts)` | `POST /api/v1/users/me/refunds` | Submit a refund request for a ticket. |
| `refunds(query=None, **opts)` | `GET /api/v1/users/me/refunds` | List the buyer's refund requests. |
| `withdraw_refund(id, body=None, **opts)` | `POST /api/v1/users/me/refunds/:id/withdraw` | Withdraw a pending refund request. |
| `create_report(body=None, **opts)` | `POST /api/v1/users/me/reports` | File a report against an event or organizer. |
| `reports(query=None, **opts)` | `GET /api/v1/users/me/reports` | List the buyer's filed reports. |
| `messages(query=None, **opts)` | `GET /api/v1/users/me/messages` | Message threads with organizers. |
| `message_thread(thread_id, query=None, **opts)` | `GET /api/v1/users/me/messages/:threadId` | A single message thread. |
| `send_ticket_message(ticket_id, body=None, **opts)` | `POST /api/v1/users/me/tickets/:ticketId/message` | Message the organizer of a ticket. |
| `notifications(query=None, **opts)` | `GET /api/v1/users/me/notifications` | In-app notifications. |
| `notifications_unread_count(query=None, **opts)` | `GET /api/v1/users/me/notifications/unread-count` | Count of unread notifications. |
| `mark_notifications_read(body=None, **opts)` | `POST /api/v1/users/me/notifications/read` | Mark notifications as read. |
| `update_avatar(body=None, **opts)` | `PUT /api/v1/users/me/avatar` | Update the account avatar. |
| `update_notification_settings(body=None, **opts)` | `PUT /api/v1/users/me/notifications/settings` | Update notification preferences. |

### `client.saved_searches` Saved searches

The buyer's saved discovery searches.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query=None, **opts)` | `GET /api/v1/users/me/saved-searches` | List saved searches. |
| `create(body=None, **opts)` | `POST /api/v1/users/me/saved-searches` | Save a search. |
| `delete(id, body=None, **opts)` | `DELETE /api/v1/users/me/saved-searches/:id` | Delete a saved search. |

### `client.data_export` Data export

GDPR-style export of the account's data.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get(query=None, **opts)` | `GET /api/v1/users/me/export` | Export all of the account's data. |

### `client.events` Events (Public)

Public event discovery rails and per-event read panels.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query=None, **opts)` | `GET /api/v1/events` | List / search public events (cursor-paginated). |
| `trending(query=None, **opts)` | `GET /api/v1/events/trending` | Trending events. |
| `categories(query=None, **opts)` | `GET /api/v1/events/categories` | Event categories with counts. |
| `nearby(query=None, **opts)` | `GET /api/v1/events/nearby` | Events near a lat/lng or city. |
| `new_this_week(query=None, **opts)` | `GET /api/v1/events/new-this-week` | Recently published events. |
| `ending_soon(query=None, **opts)` | `GET /api/v1/events/ending-soon` | Events with sales ending soon. |
| `free(query=None, **opts)` | `GET /api/v1/events/free` | Free events. |
| `recommended(query=None, **opts)` | `GET /api/v1/events/recommended` | Personalized recommendations. |
| `schedule(id, query=None, **opts)` | `GET /api/v1/events/:id/schedule` | An event's session schedule. |
| `organizer(id, query=None, **opts)` | `GET /api/v1/events/:id/organizer` | Public organizer profile for an event. |
| `faq(id, query=None, **opts)` | `GET /api/v1/events/:id/faq` | An event's FAQ. |
| `related(id, query=None, **opts)` | `GET /api/v1/events/:id/related` | Related events. |
| `express_interest(id, body=None, **opts)` | `POST /api/v1/events/:id/interest` | Register interest in an event. |
| `issue(event_id, body=None, **opts)` | `POST /api/v1/events/:eventId/issue` | Externally issue a ticket for an event (integrator mode). |
| `get(slug, query=None, **opts)` | `GET /api/v1/events/:slug` | Get a public event by slug. |
| `tickets(id, query=None, **opts)` | `GET /api/v1/events/:id/tickets` | List an event's purchasable ticket types. |

### `client.tickets` Tickets

Ticket QR/PDF, peer-to-peer transfers, promo validation and wallet passes.

| Method | Endpoint | Description |
| --- | --- | --- |
| `validate_promo(body=None, **opts)` | `POST /api/v1/tickets/promo/validate` | Validate a promo code against a cart. |
| `qr(id, query=None, **opts)` | `GET /api/v1/tickets/:id/qr` | Current rotating QR payload for a ticket (JSON). |
| `pdf(id, query=None, **opts)` | `GET /api/v1/tickets/:id/pdf` | Ticket PDF (application/pdf bytes). |
| `transfer(id, body=None, **opts)` | `POST /api/v1/tickets/:id/transfer` | Initiate a peer-to-peer transfer; the recipient gets a claim link. |
| `revoke_transfer(transfer_id, body=None, **opts)` | `POST /api/v1/tickets/transfers/:transferId/revoke` | Revoke a still-pending transfer (initiator only). |
| `get_transfer(token, query=None, **opts)` | `GET /api/v1/tickets/transfers/claim/:token` | Inspect a pending transfer by claim token. |
| `claim_transfer(token, body=None, **opts)` | `POST /api/v1/tickets/transfers/claim/:token` | Claim a transfer; rewrites the ticket holder to the recipient. |
| `wallet_pass(id, query=None, **opts)` | `GET /api/v1/tickets/:id/wallet-pass` | Google/Apple wallet 'Save to Wallet' link (JSON). |
| `list_by_event(event_id, query=None, **opts)` | `GET /api/v1/tickets/:eventId` | List ticket types for an event (legacy /ticket-types mount). |

### `client.orders` Orders

Carted checkout: create, pay, cancel, invoice.

| Method | Endpoint | Description |
| --- | --- | --- |
| `create(body=None, **opts)` | `POST /api/v1/orders` | Create an order (reserves inventory). |
| `get(id, query=None, **opts)` | `GET /api/v1/orders/:id` | Order detail. |
| `cancel(id, body=None, **opts)` | `POST /api/v1/orders/:id/cancel` | Cancel an order and release its hold. |
| `pay(id, body=None, **opts)` | `POST /api/v1/orders/:id/pay` | Initiate payment for an order (nowpayments \| paystack \| flutterwave). |
| `invoice(id, query=None, **opts)` | `GET /api/v1/orders/:id/invoice` | Order receipt PDF (application/pdf bytes). |

### `client.payments` Payments

Payment intent creation, verification, method discovery, and inbound provider webhooks.

| Method | Endpoint | Description |
| --- | --- | --- |
| `verify(body=None, **opts)` | `POST /api/v1/payments/verify` | Confirm a charge with the provider and complete the order (idempotent). |
| `get(order_id, query=None, **opts)` | `GET /api/v1/payments/:orderId` | Payment status for an order. |
| `initiate(body=None, **opts)` | `POST /api/v1/payments/initiate` | Create a payment intent. |
| `methods(query=None, **opts)` | `GET /api/v1/payments/methods` | Available payment methods for a currency. |
| `crypto_currencies(query=None, **opts)` | `GET /api/v1/payments/crypto/currencies` | Supported crypto currencies (NOWPayments). |
| `webhook_nowpayments(body=None, **opts)` | `POST /api/v1/payments/webhook/nowpayments` | Inbound NOWPayments webhook (provider callback; not for client use). |
| `webhook_paystack(body=None, **opts)` | `POST /api/v1/payments/webhook/paystack` | Inbound Paystack webhook (provider callback; not for client use). |
| `webhook_flutterwave(body=None, **opts)` | `POST /api/v1/payments/webhook/flutterwave` | Inbound Flutterwave webhook (provider callback; not for client use). |

### `client.checkin` Check-in

Gate scanning, offline manifests, batch sync, live stats and CSV export.

| Method | Endpoint | Description |
| --- | --- | --- |
| `scan(body=None, **opts)` | `POST /api/v1/checkin/scan` | Validate a QR / short-code ticket at a gate. |
| `batch(body=None, **opts)` | `POST /api/v1/checkin/batch` | Flush a queue of scans captured offline. |
| `manual(id, body=None, **opts)` | `POST /api/v1/checkin/event/:id/manual` | Manually check in an attendee. |
| `manifest(id, query=None, **opts)` | `GET /api/v1/checkin/event/:id/manifest` | Offline manifest (ticket hashes + statuses); pass ?since for a delta. |
| `stats(id, query=None, **opts)` | `GET /api/v1/checkin/event/:id/stats` | Live check-in stats snapshot. |
| `register_device(body=None, **opts)` | `POST /api/v1/checkin/device/register` | Register a scanning device. |
| `live_url(id, query=None)` | `GET /api/v1/checkin/event/:id/live` | Server-Sent Events stream of live check-in stats. |
| `gate(id, gate, query=None, **opts)` | `GET /api/v1/checkin/event/:id/gate/:gate` | Per-gate check-in stats. |
| `export(id, query=None, **opts)` | `GET /api/v1/checkin/event/:id/export` | Check-in log CSV (text/csv bytes). |

### `client.scan` Scanner-token check-in

Passwordless gate scanning using a short-lived scanner token (the /scan kiosk surface).

| Method | Endpoint | Description |
| --- | --- | --- |
| `exchange(body=None, **opts)` | `POST /api/v1/checkin-token/exchange` | Exchange a scanner token for a scoped session. |
| `session(query=None, **opts)` | `GET /api/v1/checkin-token/me` | Current scanner session (event + gate context). |
| `scan(body=None, **opts)` | `POST /api/v1/checkin-token/scan` | Validate a ticket with the scanner session. |
| `manifest(query=None, **opts)` | `GET /api/v1/checkin-token/manifest` | Offline manifest for the scanner session. |
| `batch(body=None, **opts)` | `POST /api/v1/checkin-token/batch` | Flush offline scans for the scanner session. |

### `client.search` Search

Full-text + faceted event search.

| Method | Endpoint | Description |
| --- | --- | --- |
| `query(query=None, **opts)` | `GET /api/v1/search` | Full-text + faceted search. |
| `suggest(query=None, **opts)` | `GET /api/v1/search/suggest` | Type-ahead suggestions. |
| `trending(query=None, **opts)` | `GET /api/v1/search/trending` | Trending search terms. |
| `popular(city, query=None, **opts)` | `GET /api/v1/search/popular/:city` | Popular searches in a city. |

### `client.media` Media

Image/asset uploads and the stable /media/:id resolver.

| Method | Endpoint | Description |
| --- | --- | --- |
| `upload(body=None, **opts)` | `POST /api/v1/media/upload` | Upload an image/asset (multipart/form-data). |

### `client.organizer` Organizer

Authenticated organizer surface: organizations, members, events, ticket types, schedules, sections, promo codes, attendees, payouts, refunds, reports and messaging.

| Method | Endpoint | Description |
| --- | --- | --- |
| `setup(body=None, **opts)` | `POST /api/v1/organizer/setup` | Bootstrap an organizer account + first organization. |
| `me(query=None, **opts)` | `GET /api/v1/organizer/me` | Current organizer context (orgs + memberships). |
| `create_organization(body=None, **opts)` | `POST /api/v1/organizer/organizations` | Create an organization. |
| `get_organization(org_id, query=None, **opts)` | `GET /api/v1/organizer/organizations/:orgId` | Organization detail. |
| `update_organization(org_id, body=None, **opts)` | `PUT /api/v1/organizer/organizations/:orgId` | Update an organization. |
| `set_organization_status(org_id, body=None, **opts)` | `PUT /api/v1/organizer/organizations/:orgId/status` | Change an organization's status. |
| `members(org_id, query=None, **opts)` | `GET /api/v1/organizer/organizations/:orgId/members` | List organization members. |
| `invite(org_id, body=None, **opts)` | `POST /api/v1/organizer/organizations/:orgId/invites` | Invite a member. |
| `resend_invite(org_id, member_id, body=None, **opts)` | `POST /api/v1/organizer/organizations/:orgId/invites/:memberId/resend` | Resend a member invite. |
| `remove_member(org_id, member_id, body=None, **opts)` | `DELETE /api/v1/organizer/organizations/:orgId/members/:memberId` | Remove a member. |
| `events(query=None, **opts)` | `GET /api/v1/organizer/events` | List the organizer's events. |
| `create_event(body=None, **opts)` | `POST /api/v1/organizer/events` | Create a draft event. |
| `get_event(id, query=None, **opts)` | `GET /api/v1/organizer/events/:id` | Organizer event detail. |
| `update_event(id, body=None, **opts)` | `PUT /api/v1/organizer/events/:id` | Update an event. |
| `set_event_status(id, body=None, **opts)` | `PUT /api/v1/organizer/events/:id/status` | Change an event's status. |
| `publish_event(id, body=None, **opts)` | `POST /api/v1/organizer/events/:id/publish` | Publish a draft event. |
| `unpublish_event(id, body=None, **opts)` | `POST /api/v1/organizer/events/:id/unpublish` | Unpublish a published event back to draft. |
| `delete_event(id, body=None, **opts)` | `DELETE /api/v1/organizer/events/:id` | Cancel/delete an event. |
| `tickets(id, query=None, **opts)` | `GET /api/v1/organizer/events/:id/tickets` | List an event's ticket types. |
| `create_ticket(id, body=None, **opts)` | `POST /api/v1/organizer/events/:id/tickets` | Create a ticket type. |
| `update_ticket(id, tid, body=None, **opts)` | `PUT /api/v1/organizer/events/:id/tickets/:tid` | Update a ticket type. |
| `delete_ticket(id, tid, body=None, **opts)` | `DELETE /api/v1/organizer/events/:id/tickets/:tid` | Delete a ticket type. |
| `schedule(id, query=None, **opts)` | `GET /api/v1/organizer/events/:id/schedule` | List schedule sessions. |
| `create_schedule(id, body=None, **opts)` | `POST /api/v1/organizer/events/:id/schedule` | Add a schedule session. |
| `update_schedule(id, sid, body=None, **opts)` | `PUT /api/v1/organizer/events/:id/schedule/:sid` | Update a schedule session. |
| `delete_schedule(id, sid, body=None, **opts)` | `DELETE /api/v1/organizer/events/:id/schedule/:sid` | Delete a schedule session. |
| `sections(id, query=None, **opts)` | `GET /api/v1/organizer/events/:id/sections` | List seating/venue sections. |
| `create_section(id, body=None, **opts)` | `POST /api/v1/organizer/events/:id/sections` | Add a section. |
| `update_section(id, sid, body=None, **opts)` | `PUT /api/v1/organizer/events/:id/sections/:sid` | Update a section. |
| `delete_section(id, sid, body=None, **opts)` | `DELETE /api/v1/organizer/events/:id/sections/:sid` | Delete a section. |
| `promo_codes(query=None, **opts)` | `GET /api/v1/organizer/promo-codes` | List promo codes. |
| `create_promo_code(body=None, **opts)` | `POST /api/v1/organizer/promo-codes` | Create a promo code. |
| `update_promo_code(id, body=None, **opts)` | `PUT /api/v1/organizer/promo-codes/:id` | Update a promo code. |
| `delete_promo_code(id, body=None, **opts)` | `DELETE /api/v1/organizer/promo-codes/:id` | Delete a promo code. |
| `event_analytics(id, query=None, **opts)` | `GET /api/v1/organizer/events/:id/analytics` | Sales/analytics for an event. |
| `event_attendees(id, query=None, **opts)` | `GET /api/v1/organizer/events/:id/attendees` | Attendee CRM list for an event. |
| `event_export(id, query=None, **opts)` | `GET /api/v1/organizer/events/:id/export` | Attendee export CSV (text/csv bytes). |
| `payouts(query=None, **opts)` | `GET /api/v1/organizer/payouts` | List payouts. |
| `payout(id, query=None, **opts)` | `GET /api/v1/organizer/payouts/:id` | Payout detail. |
| `request_payout(body=None, **opts)` | `POST /api/v1/organizer/payouts/request` | Request a payout. |
| `update_payout_settings(body=None, **opts)` | `PUT /api/v1/organizer/payout-settings` | Update payout settings. |
| `refunds(org_id, query=None, **opts)` | `GET /api/v1/organizer/orgs/:orgId/refunds` | List refund requests for an org. |
| `decide_refund(id, body=None, **opts)` | `POST /api/v1/organizer/refunds/:id/decide` | Approve or deny a refund request. |
| `reports(org_id, query=None, **opts)` | `GET /api/v1/organizer/orgs/:orgId/reports` | List reports filed against an org (read-only). |
| `resolve_report(id, body=None, **opts)` | `POST /api/v1/organizer/reports/:id/resolve` | Resolve a report (admins only; organizers receive 403). |
| `messages(org_id, query=None, **opts)` | `GET /api/v1/organizer/orgs/:orgId/messages` | Org-wide buyer message threads. |
| `overview(org_id, query=None, **opts)` | `GET /api/v1/organizer/orgs/:orgId/overview` | Org dashboard overview. |
| `referral(org_id, query=None, **opts)` | `GET /api/v1/organizer/orgs/:orgId/referral` | Referral program summary + commissions. |
| `notifications(org_id, query=None, **opts)` | `GET /api/v1/organizer/orgs/:orgId/notifications` | Org notifications. |
| `fund_wallet(org_id, body=None, **opts)` | `POST /api/v1/organizer/wallets/org/:orgId/fund` | Top up the org wallet (returns a payment intent). |
| `fund_wallet_status(org_id, order_id, query=None, **opts)` | `GET /api/v1/organizer/wallets/org/:orgId/fund/:orderId` | Poll a wallet top-up payment. |
| `payout_details(org_id, query=None, **opts)` | `GET /api/v1/organizer/orgs/:orgId/payout-details` | Stored payout destinations. |
| `update_payout_details(org_id, currency, body=None, **opts)` | `PUT /api/v1/organizer/orgs/:orgId/payout-details/:currency` | Set payout details for a currency. |
| `ticket_message(ticket_id, body=None, **opts)` | `POST /api/v1/organizer/tickets/:ticketId/message` | Reply to a buyer on a ticket thread. |
| `message_thread(thread_id, query=None, **opts)` | `GET /api/v1/organizer/messages/:threadId` | A single org message thread. |

### `client.wallets` Wallets

Organization wallet balances and ledger.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query=None, **opts)` | `GET /api/v1/organizer/wallets` | List wallets the caller can see. |
| `get(org_id, query=None, **opts)` | `GET /api/v1/organizer/wallets/org/:orgId` | An org's wallet balances (per currency). |
| `bootstrap(org_id, body=None, **opts)` | `POST /api/v1/organizer/wallets/org/:orgId/bootstrap` | Provision an org's wallet. |
| `transactions(org_id, query=None, **opts)` | `GET /api/v1/organizer/wallets/org/:orgId/transactions` | Wallet ledger transactions. |

### `client.scanner_tokens` Scanner tokens

Manage short-lived gate-scanner tokens for staff devices.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(org_id, query=None, **opts)` | `GET /api/v1/organizer/scanner-tokens/org/:orgId` | List scanner tokens. |
| `create(org_id, body=None, **opts)` | `POST /api/v1/organizer/scanner-tokens/org/:orgId` | Mint a scanner token. |
| `update(org_id, token_id, body=None, **opts)` | `PUT /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId` | Update a scanner token. |
| `delete(org_id, token_id, body=None, **opts)` | `DELETE /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId` | Revoke a scanner token. |
| `reissue(org_id, token_id, body=None, **opts)` | `POST /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId/reissue` | Reissue a scanner token's secret. |
| `metrics(org_id, token_id, query=None, **opts)` | `GET /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId/metrics` | Usage metrics for a scanner token. |

### `client.integrations` Integrations

API keys, MCP tokens, and integration usage metrics.

| Method | Endpoint | Description |
| --- | --- | --- |
| `metrics(org_id, query=None, **opts)` | `GET /api/v1/organizer/integrations/org/:orgId/metrics` | Integration usage metrics. |
| `api_calls(org_id, query=None, **opts)` | `GET /api/v1/organizer/integrations/org/:orgId/api-calls` | Recent API call log. |
| `mcp_calls(org_id, query=None, **opts)` | `GET /api/v1/organizer/integrations/org/:orgId/mcp-calls` | Recent MCP tool-call log. |
| `list_api_keys(org_id, query=None, **opts)` | `GET /api/v1/organizer/integrations/org/:orgId/api-keys` | List API keys. |
| `create_api_key(org_id, body=None, **opts)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys` | Mint an API key (plaintext returned once). |
| `update_api_key(org_id, key_id, body=None, **opts)` | `PUT /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Update an API key (scopes, status, allowlist). |
| `rotate_api_key(org_id, key_id, body=None, **opts)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId/rotate` | Rotate an API key's secret. |
| `delete_api_key(org_id, key_id, body=None, **opts)` | `DELETE /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Revoke an API key. |
| `list_mcp_tokens(org_id, query=None, **opts)` | `GET /api/v1/organizer/integrations/org/:orgId/mcp-tokens` | List MCP tokens. |
| `create_mcp_token(org_id, body=None, **opts)` | `POST /api/v1/organizer/integrations/org/:orgId/mcp-tokens` | Mint an MCP token. |
| `update_mcp_token(org_id, token_id, body=None, **opts)` | `PUT /api/v1/organizer/integrations/org/:orgId/mcp-tokens/:tokenId` | Update an MCP token. |
| `delete_mcp_token(org_id, token_id, body=None, **opts)` | `DELETE /api/v1/organizer/integrations/org/:orgId/mcp-tokens/:tokenId` | Revoke an MCP token. |

### `client.event_customization` Event customization

Per-event white-label theming.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get(event_id, query=None, **opts)` | `GET /api/v1/organizer/event-customization/:eventId` | Get an event's customization. |
| `update(event_id, body=None, **opts)` | `PUT /api/v1/organizer/event-customization/:eventId` | Update an event's customization. |

### `client.growth` Growth

Comp tickets, CSV import, broadcasts and attendee tags.

| Method | Endpoint | Description |
| --- | --- | --- |
| `mint_comps(event_id, body=None, **opts)` | `POST /api/v1/organizer/growth/events/:eventId/comps` | Issue complimentary tickets. |
| `import_comps_csv(event_id, body=None, **opts)` | `POST /api/v1/organizer/growth/events/:eventId/comps/import-csv` | Bulk-issue comps from CSV. |
| `list_comps(event_id, query=None, **opts)` | `GET /api/v1/organizer/growth/events/:eventId/comps` | List issued comps. |
| `resend_comp(event_id, ticket_id, body=None, **opts)` | `POST /api/v1/organizer/growth/events/:eventId/comps/:ticketId/resend` | Resend a comp ticket email. |
| `broadcast_event(event_id, body=None, **opts)` | `POST /api/v1/organizer/growth/events/:eventId/broadcast` | Broadcast to an event's attendees. |
| `broadcast_org(org_id, body=None, **opts)` | `POST /api/v1/organizer/growth/orgs/:orgId/broadcast` | Broadcast to an org's followers/subscribers. |
| `list_broadcasts(org_id, query=None, **opts)` | `GET /api/v1/organizer/growth/orgs/:orgId/broadcasts` | List sent broadcasts. |
| `add_tags(body=None, **opts)` | `POST /api/v1/organizer/growth/tags` | Tag attendees (body: orgId, ticketIds, tag). |
| `remove_tag(body=None, **opts)` | `DELETE /api/v1/organizer/growth/tags` | Remove a tag (body: orgId, ticketId, tag). |
| `list_tags(org_id, query=None, **opts)` | `GET /api/v1/organizer/growth/orgs/:orgId/tags` | List tags for an org. |
| `tag_attendees(org_id, tag, query=None, **opts)` | `GET /api/v1/organizer/growth/orgs/:orgId/tags/:tag/attendees` | List attendees with a given tag. |

### `client.public_events` Public events (vanity)

Public org/event read endpoints used by hosted pages and white-label sites.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get_by_slug(slug, query=None, **opts)` | `GET /api/v1/public/events/:slug` | Public event by slug. |
| `org_events(org_id, query=None, **opts)` | `GET /api/v1/public/events/orgs/by/:orgId` | An organization's public events. |
| `get_by_org_event(org_id, event_id, query=None, **opts)` | `GET /api/v1/public/events/by/:orgId/:eventId` | Public event by org + event id. |
| `get_by_id(event_id, query=None, **opts)` | `GET /api/v1/public/events/by-id/:eventId` | Public event by id. |
| `preview(event_id, query=None, **opts)` | `GET /api/v1/public/events/preview/:eventId` | Draft event preview (token-gated). |

### `client.site` Public site

Sitemap, newsletter signup and platform status.

| Method | Endpoint | Description |
| --- | --- | --- |
| `sitemap(query=None, **opts)` | `GET /api/v1/public/sitemap.xml` | Sitemap XML (application/xml bytes). |
| `newsletter_start(body=None, **opts)` | `POST /api/v1/public/newsletter/start` | Begin newsletter double-opt-in. |
| `newsletter_confirm(body=None, **opts)` | `POST /api/v1/public/newsletter` | Confirm a newsletter subscription. |
| `status(query=None, **opts)` | `GET /api/v1/public/status` | Platform status + incidents. |

### `client.community` Community

Verified-attendee reviews, organizer follows and event waitlists.

| Method | Endpoint | Description |
| --- | --- | --- |
| `submit_review(body=None, **opts)` | `POST /api/v1/community/reviews` | Submit a verified-attendee review (ticketCode + email prove ownership). |
| `org_reviews(org_id, query=None, **opts)` | `GET /api/v1/community/orgs/:orgId/reviews` | Published reviews for an org. |
| `event_reviews(event_id, query=None, **opts)` | `GET /api/v1/community/events/:eventId/reviews` | Published reviews for an event. |
| `follow(org_id, body=None, **opts)` | `POST /api/v1/community/orgs/:orgId/follow` | Follow an organizer. |
| `unfollow(token, query=None, **opts)` | `GET /api/v1/community/unfollow/:token` | Unsubscribe via emailed token. |
| `join_waitlist(event_id, body=None, **opts)` | `POST /api/v1/community/events/:eventId/waitlist` | Join an event waitlist. |
| `accept_waitlist(id, token, query=None, **opts)` | `GET /api/v1/community/waitlist/accept/:id/:token` | Accept a waitlist offer via emailed token. |
| `manage_reviews(org_id, query=None, **opts)` | `GET /api/v1/community/orgs/:orgId/reviews/manage` | Organizer view of all reviews (incl. pending). |
| `reply_review(id, body=None, **opts)` | `POST /api/v1/community/reviews/:id/reply` | Organizer reply to a review. |
| `set_review_status(id, body=None, **opts)` | `POST /api/v1/community/reviews/:id/status` | Publish/hide a review. |
| `followers(org_id, query=None, **opts)` | `GET /api/v1/community/orgs/:orgId/followers` | List an org's followers. |
| `remove_follower(org_id, follower_id, body=None, **opts)` | `DELETE /api/v1/community/orgs/:orgId/followers/:followerId` | Remove a follower. |
| `event_waitlist(event_id, query=None, **opts)` | `GET /api/v1/community/events/:eventId/waitlist` | List an event's waitlist. |
| `offer_waitlist(event_id, body=None, **opts)` | `POST /api/v1/community/events/:eventId/waitlist/offer` | Offer spots to waitlisted attendees. |

### `client.track` Tracking

Lightweight page-view tracking.

| Method | Endpoint | Description |
| --- | --- | --- |
| `view(body=None, **opts)` | `POST /api/v1/track/view` | Record a page view. |

### `client.webhooks` Webhooks

Register webhook endpoints and inspect/replay deliveries. (Use client.webhooks.verify() to validate inbound signatures.)

| Method | Endpoint | Description |
| --- | --- | --- |
| `catalog(query=None, **opts)` | `GET /api/v1/webhooks/catalog` | List subscribable event types. |
| `list(query=None, **opts)` | `GET /api/v1/webhooks` | List registered webhook endpoints. |
| `create(body=None, **opts)` | `POST /api/v1/webhooks` | Register a webhook endpoint. |
| `update(id, body=None, **opts)` | `PUT /api/v1/webhooks/:id` | Update a webhook endpoint. |
| `rotate_secret(id, body=None, **opts)` | `POST /api/v1/webhooks/:id/rotate-secret` | Rotate a webhook signing secret. |
| `delete(id, body=None, **opts)` | `DELETE /api/v1/webhooks/:id` | Delete a webhook endpoint. |
| `deliveries(id, query=None, **opts)` | `GET /api/v1/webhooks/:id/deliveries` | List delivery attempts for an endpoint. |
| `test(id, body=None, **opts)` | `POST /api/v1/webhooks/:id/test` | Send a test event to an endpoint. |
| `replay(id, body=None, **opts)` | `POST /api/v1/webhooks/deliveries/:id/replay` | Replay a past delivery. |

### `client.white_label` White-label

Self-contained white-label surface: events, ticket types, customization, orders, tickets and check-in under one integration.

| Method | Endpoint | Description |
| --- | --- | --- |
| `me(query=None, **opts)` | `GET /api/v1/white-label/me` | The integration's white-label context. |
| `events(query=None, **opts)` | `GET /api/v1/white-label/events` | List white-label events. |
| `create_event(body=None, **opts)` | `POST /api/v1/white-label/events` | Create a white-label event. |
| `get_event(id, query=None, **opts)` | `GET /api/v1/white-label/events/:id` | White-label event detail. |
| `update_event(id, body=None, **opts)` | `PUT /api/v1/white-label/events/:id` | Update a white-label event. |
| `delete_event(id, body=None, **opts)` | `DELETE /api/v1/white-label/events/:id` | Delete a white-label event. |
| `ticket_types(event_id, query=None, **opts)` | `GET /api/v1/white-label/events/:eventId/ticket-types` | List ticket types. |
| `create_ticket_type(event_id, body=None, **opts)` | `POST /api/v1/white-label/events/:eventId/ticket-types` | Create a ticket type. |
| `get_customization(event_id, query=None, **opts)` | `GET /api/v1/white-label/events/:eventId/customization` | Get event customization. |
| `update_customization(event_id, body=None, **opts)` | `PUT /api/v1/white-label/events/:eventId/customization` | Update event customization. |
| `orders(query=None, **opts)` | `GET /api/v1/white-label/orders` | List white-label orders. |
| `tickets(query=None, **opts)` | `GET /api/v1/white-label/tickets` | List white-label tickets. |
| `checkin_scan(body=None, **opts)` | `POST /api/v1/white-label/checkin/scan` | Validate a ticket at a white-label gate. |
| `checkin_stats(event_id, query=None, **opts)` | `GET /api/v1/white-label/checkin/stats/:eventId` | White-label check-in stats. |
| `wallets(query=None, **opts)` | `GET /api/v1/white-label/wallets` | White-label wallet balances. |

### `client.support` Support

Support tickets and threaded messages.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query=None, **opts)` | `GET /api/v1/support` | List support tickets. |
| `create(body=None, **opts)` | `POST /api/v1/support` | Open a support ticket. |
| `get(id, query=None, **opts)` | `GET /api/v1/support/:id` | Support ticket detail. |
| `send_message(id, body=None, **opts)` | `POST /api/v1/support/:id/messages` | Reply on a support ticket. |

### `client.util` Utilities

Helper endpoints.

| Method | Endpoint | Description |
| --- | --- | --- |
| `resolve_coords(body=None, **opts)` | `POST /api/v1/util/resolve-coords` | Resolve an address to coordinates. |

<!-- END ENDPOINTS -->
