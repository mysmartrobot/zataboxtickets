# zatabox Python SDK

Official **Python SDK** for the [Zatabox Tickets](https://zatabox.com) REST API the
white-label event-ticketing platform. A small, dependency-free client over
`https://api.zatabox.com/api/v1` that handles auth, sandbox routing, idempotency,
retries, pagination, live (SSE) streaming and webhook verification.

- **Zero dependencies** standard library only (`urllib`, `hmac`, `hashlib`).
- **Complete** every one of the **78 REST endpoints** is a method.
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
- [Live check-in stream (SSE)](#live-check-in-stream-sse)
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

This SDK is **distributed via GitHub** it is not published to PyPI. The package lives
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
with your production account). A test key used against production or vice-versa 
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

`auth`, `events`, `organizer`, `event_customization`, `tickets`, `orders`, `payments`,
`checkin`, `community`, `growth`, `users`, `integrations`, `webhooks`.

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
z.organizer.update_schedule(event_id, session_id, {"sessionTitle": "Keynote"})  # two params + body
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
for page in z.paginate(z.events.list, {"q": "jazz", "limit": 50}):
    for ev in page["items"]:
        print(ev["id"])
```

`paginate(list_method, query)` accepts any cursor-paginated list method (e.g.
`events.list`, `users.tickets`, `webhooks.deliveries`) and follows the cursor across
both response shapes. To page manually:

```python
cursor = None
while True:
    page = z.events.list({"limit": 50, "cursor": cursor})
    ...
    cursor = (page.get("pagination") or {}).get("cursor") or page.get("nextCursor")
    if not cursor:
        break
```

## Live check-in stream (SSE)

`checkin.live_url(event_id)` returns the stream URL; consume it with your preferred SSE
client (passing `Authorization: Bearer <key>`):

```python
url = z.checkin.live_url(event_id)
# e.g. with `sseclient`/`httpx` each `stats` event carries a JSON snapshot.
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

The SDK exposes **78 endpoints** across **13 namespaces**. Every method is listed below with its idiomatic signature, the underlying HTTP route, and what it does. Path parameters are positional; reads take an optional query map and writes take an optional body, both followed by a call-options bag.

### `client.auth` Auth

Account registration, password + passwordless sign-in, 2FA login and token refresh.

| Method | Endpoint | Description |
| --- | --- | --- |
| `register(body=None, **opts)` | `POST /api/v1/auth/register` | Register an account; returns the user plus an accessToken/refreshToken pair. |
| `login(body=None, **opts)` | `POST /api/v1/auth/login` | Log in with email + password; returns a JWT pair (or a 2FA challenge). |
| `login_verify2fa(body=None, **opts)` | `POST /api/v1/auth/2fa-verify` | Complete a 2FA login challenge; returns the JWT pair. |
| `request_token(body=None, **opts)` | `POST /api/v1/auth/token/request` | Passwordless: email a buyer a 6-digit login code. |
| `exchange_token(body=None, **opts)` | `POST /api/v1/auth/token/exchange` | Passwordless: exchange email + 6-digit code for a JWT pair. |
| `refresh(body=None, **opts)` | `POST /api/v1/auth/refresh` | Refresh an expired access token (rotates the refresh token). |
| `logout(body=None, **opts)` | `POST /api/v1/auth/logout` | Revoke a refresh token. |

### `client.events` Events (Public)

Public event discovery and read, plus external ticket issuance.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query=None, **opts)` | `GET /api/v1/events` | List and search published public events (cursor-paginated). |
| `get(slug, query=None, **opts)` | `GET /api/v1/events/:slug` | Event detail by slug (organizer info, schedule, active ticket types). |
| `tickets(id, query=None, **opts)` | `GET /api/v1/events/:id/tickets` | List an event's ticket types with live availability. |
| `issue(event_id, body=None, **opts)` | `POST /api/v1/events/:eventId/issue` | Issue tickets you sold elsewhere (developer-handled payment; 3% wallet fee on paid tickets). |

### `client.organizer` Organizer

Organizer surface: organization read, events, ticket types, schedule sessions, seating sections and promo codes.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get_organization(id, query=None, **opts)` | `GET /api/v1/organizer/organizations/:id` | Get organization details and per-currency wallet balances. |
| `create_event(body=None, **opts)` | `POST /api/v1/organizer/events` | Create a draft event. |
| `update_event(id, body=None, **opts)` | `PUT /api/v1/organizer/events/:id` | Partial-update an event. |
| `publish_event(id, body=None, **opts)` | `POST /api/v1/organizer/events/:id/publish` | Publish a draft event. |
| `unpublish_event(id, body=None, **opts)` | `POST /api/v1/organizer/events/:id/unpublish` | Unpublish a published event back to draft. |
| `delete_event(id, body=None, **opts)` | `DELETE /api/v1/organizer/events/:id` | Cancel an event. |
| `create_ticket(id, body=None, **opts)` | `POST /api/v1/organizer/events/:id/tickets` | Create a ticket type. |
| `schedule(id, query=None, **opts)` | `GET /api/v1/organizer/events/:id/schedule` | List schedule sessions (running order). |
| `create_schedule(id, body=None, **opts)` | `POST /api/v1/organizer/events/:id/schedule` | Add a schedule session. |
| `update_schedule(id, session_id, body=None, **opts)` | `PUT /api/v1/organizer/events/:id/schedule/:sessionId` | Update a schedule session. |
| `delete_schedule(id, session_id, body=None, **opts)` | `DELETE /api/v1/organizer/events/:id/schedule/:sessionId` | Delete a schedule session. |
| `sections(id, query=None, **opts)` | `GET /api/v1/organizer/events/:id/sections` | List seating/capacity sections. |
| `create_section(id, body=None, **opts)` | `POST /api/v1/organizer/events/:id/sections` | Add a seating section. |
| `update_section(id, section_id, body=None, **opts)` | `PUT /api/v1/organizer/events/:id/sections/:sectionId` | Update a seating section. |
| `delete_section(id, section_id, body=None, **opts)` | `DELETE /api/v1/organizer/events/:id/sections/:sectionId` | Delete a seating section. |
| `promo_codes(query=None, **opts)` | `GET /api/v1/organizer/promo-codes` | List promo codes (optionally filtered by event). |
| `create_promo_code(body=None, **opts)` | `POST /api/v1/organizer/promo-codes` | Create a promo code. |
| `update_promo_code(id, body=None, **opts)` | `PUT /api/v1/organizer/promo-codes/:id` | Update a promo code. |
| `delete_promo_code(id, body=None, **opts)` | `DELETE /api/v1/organizer/promo-codes/:id` | Delete or disable a promo code. |

### `client.event_customization` Event page customization

Per-event public-page theming and the “Good to know” FAQ.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get(id, query=None, **opts)` | `GET /api/v1/organizer/event-customization/:id` | Get an event's page customization (theme, layout, FAQ, SEO). |
| `update(id, body=None, **opts)` | `PUT /api/v1/organizer/event-customization/:id` | Update an event's page customization (incl. the FAQ list). |

### `client.tickets` Tickets

Checkout-time ticket helpers.

| Method | Endpoint | Description |
| --- | --- | --- |
| `validate_promo(body=None, **opts)` | `POST /api/v1/tickets/promo/validate` | Validate a promo code against a cart (read-only preview, does not consume a use). |

### `client.orders` Orders

Carted checkout: create, read, pay, cancel.

| Method | Endpoint | Description |
| --- | --- | --- |
| `create(body=None, **opts)` | `POST /api/v1/orders` | Create an order (guest checkout needs only name + email). |
| `get(id, query=None, **opts)` | `GET /api/v1/orders/:id` | Get an order (pass ?token for guest reads). |
| `pay(id, body=None, **opts)` | `POST /api/v1/orders/:id/pay` | Initiate payment (provider: nowpayments \| paystack \| flutterwave). |
| `cancel(id, body=None, **opts)` | `POST /api/v1/orders/:id/cancel` | Cancel an unpaid order and release held inventory. |

### `client.payments` Payments

Verify charges, read payment status, list crypto coins.

| Method | Endpoint | Description |
| --- | --- | --- |
| `verify(body=None, **opts)` | `POST /api/v1/payments/verify` | Actively verify a payment with the provider and issue tickets (idempotent, poll-safe). |
| `get(order_id, query=None, **opts)` | `GET /api/v1/payments/:orderId` | Read payment/order status and attempts (read-only). |
| `crypto_currencies(query=None, **opts)` | `GET /api/v1/payments/crypto/currencies` | List supported NOWPayments crypto coins (for the payCurrency value). |

### `client.checkin` Check-in

Gate scanning, offline manifests + sync, live stats.

| Method | Endpoint | Description |
| --- | --- | --- |
| `scan(body=None, **opts)` | `POST /api/v1/checkin/scan` | Validate a QR, barcode or 6-character door code at the gate. |
| `manual(id, body=None, **opts)` | `POST /api/v1/checkin/event/:id/manual` | Manually check in a typed ticket code. |
| `manifest(id, query=None, **opts)` | `GET /api/v1/checkin/event/:id/manifest` | Hashed guest-list manifest for offline scanning (pass ?since for a delta). |
| `batch(body=None, **opts)` | `POST /api/v1/checkin/batch` | Sync up to 500 queued offline scans. |
| `stats(id, query=None, **opts)` | `GET /api/v1/checkin/event/:id/stats` | Check-in totals, capacity %, entry rate and per-gate breakdown. |
| `gate(id, gate, query=None, **opts)` | `GET /api/v1/checkin/event/:id/gate/:gate` | Per-gate check-in stats slice. |
| `live_url(id, query=None)` | `GET /api/v1/checkin/event/:id/live` | Server-Sent Events stream a stats snapshot every 2 seconds. |

### `client.community` Community

Verified-attendee reviews, organizer follows/subscribers and event waitlists.

| Method | Endpoint | Description |
| --- | --- | --- |
| `submit_review(body=None, **opts)` | `POST /api/v1/community/reviews` | Review an event (checked-in ticket holders only; ticketCode + email prove attendance). |
| `follow(org_id, body=None, **opts)` | `POST /api/v1/community/orgs/:orgId/follow` | Follow an organizer (subscribe to new-event announcements). |
| `followers(org_id, query=None, **opts)` | `GET /api/v1/community/orgs/:orgId/followers` | List an organizer's subscribers (organizer auth). |
| `remove_follower(org_id, follower_id, body=None, **opts)` | `DELETE /api/v1/community/orgs/:orgId/followers/:followerId` | Remove a subscriber (organizer auth). |
| `join_waitlist(event_id, body=None, **opts)` | `POST /api/v1/community/events/:eventId/waitlist` | Join an event waitlist (offers fire on cancellations). |

### `client.growth` Growth (Organizer)

Comp tickets, CSV import, broadcasts and attendee tags.

| Method | Endpoint | Description |
| --- | --- | --- |
| `mint_comps(event_id, body=None, **opts)` | `POST /api/v1/organizer/growth/events/:eventId/comps` | Bulk-mint and email complimentary tickets. |
| `import_comps_csv(event_id, body=None, **opts)` | `POST /api/v1/organizer/growth/events/:eventId/comps/import-csv` | Import attendees (comp tickets) from CSV. |
| `broadcast_event(event_id, body=None, **opts)` | `POST /api/v1/organizer/growth/events/:eventId/broadcast` | Email a broadcast to an event's attendees (replies thread to the organizer inbox). |
| `add_tags(body=None, **opts)` | `POST /api/v1/organizer/growth/tags` | Tag attendees (additive; powers broadcast filters and CRM segments). |
| `remove_tag(body=None, **opts)` | `DELETE /api/v1/organizer/growth/tags` | Remove an attendee tag. |

### `client.users` Buyers

The authenticated buyer: profile, ticket wallet, data export, refunds, reports and organizer messaging.

| Method | Endpoint | Description |
| --- | --- | --- |
| `me(query=None, **opts)` | `GET /api/v1/users/me` | Current buyer profile. |
| `tickets(query=None, **opts)` | `GET /api/v1/users/me/tickets` | The buyer's ticket wallet (cursor-paginated). |
| `export(query=None, **opts)` | `GET /api/v1/users/me/export` | GDPR data export one JSON download of everything on the account. |
| `create_refund(body=None, **opts)` | `POST /api/v1/users/me/refunds` | Request a refund for a ticket. |
| `create_report(body=None, **opts)` | `POST /api/v1/users/me/reports` | File a report against an event or organizer. |
| `messages(query=None, **opts)` | `GET /api/v1/users/me/messages` | The buyer's message threads with organizers. |
| `send_ticket_message(ticket_id, body=None, **opts)` | `POST /api/v1/users/me/tickets/:ticketId/message` | Message the organizer about a ticket (rate-limited). |

### `client.integrations` API keys

Manage your organization's own API keys (organizer owner/admin auth).

| Method | Endpoint | Description |
| --- | --- | --- |
| `create_api_key(org_id, body=None, **opts)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys` | Create an API key (plaintext secret returned exactly once). |
| `list_api_keys(org_id, query=None, **opts)` | `GET /api/v1/organizer/integrations/org/:orgId/api-keys` | List API keys (prefixes and metadata only, never the secret). |
| `update_api_key(org_id, key_id, body=None, **opts)` | `PUT /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Update a key (rename, pause, re-scope). |
| `rotate_api_key(org_id, key_id, body=None, **opts)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId/rotate` | Rotate a key's secret (new secret returned once; old one invalidated). |
| `delete_api_key(org_id, key_id, body=None, **opts)` | `DELETE /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Revoke an API key. |

### `client.webhooks` Webhooks

Register webhook endpoints, manage secrets, inspect and replay deliveries. (Use webhooks.verify() to validate inbound signatures.)

| Method | Endpoint | Description |
| --- | --- | --- |
| `create(body=None, **opts)` | `POST /api/v1/webhooks` | Create a webhook endpoint (signing secret returned exactly once). |
| `list(query=None, **opts)` | `GET /api/v1/webhooks` | List webhook endpoints. |
| `update(id, body=None, **opts)` | `PUT /api/v1/webhooks/:id` | Update a webhook endpoint. |
| `delete(id, body=None, **opts)` | `DELETE /api/v1/webhooks/:id` | Delete a webhook endpoint. |
| `test(id, body=None, **opts)` | `POST /api/v1/webhooks/:id/test` | Send a signed test event to the endpoint. |
| `rotate_secret(id, body=None, **opts)` | `POST /api/v1/webhooks/:id/rotate-secret` | Rotate the signing secret (new secret returned once). |
| `deliveries(id, query=None, **opts)` | `GET /api/v1/webhooks/:id/deliveries` | List delivery attempts for an endpoint. |
| `replay(id, body=None, **opts)` | `POST /api/v1/webhooks/deliveries/:id/replay` | Replay a past delivery. |
| `catalog(query=None, **opts)` | `GET /api/v1/webhooks/catalog` | List every subscribable event type (no auth). |

<!-- END ENDPOINTS -->
