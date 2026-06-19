# zatabox Ruby SDK

Official **Ruby SDK** for the [Zatabox Tickets](https://zatabox.com) REST API the
white-label event-ticketing platform. A small, dependency-free client over
`https://api.zatabox.com/api/v1` that handles auth, sandbox routing, idempotency,
retries, pagination, live (SSE) streaming and webhook verification.

- **Zero gem dependencies** standard library only (`net/http`, `openssl`, `json`).
- **Complete** every one of the **78 REST endpoints** is a method.
- **Generated, never drifts** emitted from the canonical
  [`endpoints.json`](../spec/endpoints.json) spec.
- **Ruby 2.5+**.

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

- **Ruby 2.5 or newer**. No gem dependencies.
- A Zatabox **API key** (`vt_live_…` / `vt_test_…`), a **portal JWT**, or an **MCP
  token**. Mint API keys in the organizer portal → Integrations, or the
  [sandbox console](https://tester.zatabox.com).

## Installation

This SDK is **distributed via GitHub** it is not published to RubyGems. The gem lives
in the `ruby/` directory of
[`mysmartrobot/zataboxtickets`](https://github.com/mysmartrobot/zataboxtickets). Point
Bundler at the repo and the subdirectory gemspec with `glob:`:

```ruby
# Gemfile
gem "zatabox", git: "https://github.com/mysmartrobot/zataboxtickets.git", glob: "ruby/*.gemspec"
```

Pin to a tag or branch for reproducible builds:

```ruby
gem "zatabox", git: "https://github.com/mysmartrobot/zataboxtickets.git",
               glob: "ruby/*.gemspec", tag: "v0.3.0"
```

Then `bundle install`. Without Bundler, clone the repo and add `ruby/lib` to the load
path (`ruby -I/path/to/zataboxtickets/ruby/lib your_script.rb`). The SDK has **zero gem
dependencies**. Then:

```ruby
require "zatabox"
```

## Quick start

```ruby
require "zatabox"

# A vt_test_ key auto-routes to the sandbox; a vt_live_ key to production.
z = Zatabox::Client.new(api_key: ENV.fetch("ZATABOX_API_KEY"))

begin
  event = z.organizer.create_event(
    title: "Warehouse Sessions 004",
    category: "music",
    startDate: "2026-08-22T20:00:00Z",
    endDate: "2026-08-23T02:00:00Z",
    timezone: "Africa/Lagos",
    venueType: "physical",
    venueCity: "Lagos",
    capacity: 450
  )
  z.organizer.create_ticket(event["id"],
    name: "General Admission", type: "general", price: 5000, currency: "NGN",
    quantityTotal: 450, saleStart: "2026-07-01T00:00:00Z", saleEnd: "2026-08-22T20:00:00Z")
  z.organizer.publish_event(event["id"])
  puts "published: #{z.events.get(event['slug'])['status']}"
rescue Zatabox::Error => e
  warn "#{e.code} #{e.message} #{e.request_id}"
end
```

## Authentication

The SDK forwards one `Authorization: Bearer <token>` header. Three ways to authenticate:

```ruby
Zatabox::Client.new(api_key: "vt_live_...")   # scoped API key (prefix selects env)
Zatabox::Client.new(api_key: "vt_test_...")   # sandbox (auto-routed)
Zatabox::Client.new(bearer_token: "eyJ...")   # portal JWT or vt_mcp_ token
Zatabox::Client.new(api_key: "vt_test_...", base_url: "http://localhost:4100")
```

### Passwordless buyer login

```ruby
anon = Zatabox::Client.new(bearer_token: "unused", base_url: "https://api.zatabox.com")
anon.auth.request_token(email: "fan@example.com")               # emails a 6-digit code
session = anon.auth.exchange_token(email: "fan@example.com", code: "123456")

buyer = Zatabox::Client.new(bearer_token: session["accessToken"])
tickets = buyer.users.tickets
```

### Refreshing & swapping tokens

```ruby
nxt = buyer.auth.refresh(refreshToken: session["refreshToken"])
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

```ruby
Zatabox::Client.new(api_key: "vt_test_...")   # → sandbox.zatabox.com
Zatabox::Client.new(api_key: "vt_live_...")   # → api.zatabox.com
Zatabox::Client.new(api_key: "vt_test_...", base_url: "http://localhost:4100")
```

Mint/rotate `vt_test_` keys, watch **live request logs**, see usage and browse the
endpoint catalog in the **sandbox console at https://tester.zatabox.com** (sign in
with your production account). A test key used against production or vice-versa 
returns `403 WRONG_ENV`.

## Client configuration

```ruby
Zatabox::Client.new(
  api_key: "vt_live_...",   # or bearer_token:
  base_url: nil,            # override the auto-resolved host
  timeout: 30,              # per-request timeout (seconds)
  max_retries: 2,           # retries for 5xx / network / timeout
  user_agent: nil           # defaults to zatabox-ruby/<version>
)
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `api_key` | `String` | | `vt_live_…` / `vt_test_…`; test keys auto-route to the sandbox. |
| `bearer_token` | `String` | | Portal JWT / `vt_mcp_…`. One credential is required. |
| `base_url` | `String` | resolved from key | Explicit API origin; wins over prefix routing. |
| `timeout` | `Integer` | `30` | Per-request open/read timeout (seconds). |
| `max_retries` | `Integer` | `2` | Retries for `5xx`/network/timeout (never `4xx`). |
| `user_agent` | `String` | `zatabox-ruby/<version>` | Overrides the `User-Agent` header. |

## How methods map to endpoints

`client.<namespace>.<method>(...)`. Namespaces are snake_case:

`auth`, `events`, `organizer`, `event_customization`, `tickets`, `orders`, `payments`,
`checkin`, `community`, `growth`, `users`, `integrations`, `webhooks`.

Argument order:

```
method(path_param1, path_param2, …, payload = nil, opts = {})
```

- **Path params** first (URL-encoded for you).
- **Reads** take an optional `query` Hash; **writes** take an optional `body` Hash.
- **`opts`** is a trailing Hash: `idempotency_key:`, `headers:`, and (for writes) an
  extra `query:`.

```ruby
z.events.list(q: "jazz", city: "Lagos", limit: 20)               # query
z.events.get("warehouse-sessions-004")                           # path param
z.organizer.update_schedule(event_id, session_id, sessionTitle: "Keynote")  # two params + body
z.orders.create({ items: items }, idempotency_key: my_uuid)      # body + opts
```

> Tip: a brace-less trailing Hash binds to the **body/query**, so
> `z.orders.create(items: [...])` works; pass options as a second Hash:
> `z.orders.create({ items: [...] }, idempotency_key: key)`.

## Responses

The API wraps success as `{ "success", "data", "meta" }`; the SDK **returns `data`
directly** (a Hash/Array with string keys). Pagination cursors and `request_id` live
inside that document.

```ruby
page = z.events.list(limit: 20)   # e.g. { "items" => [...], "pagination" => {...} }
```

## Error handling

Any non-2xx (and network/timeout/webhook failures) raises **`Zatabox::Error`**:

```ruby
begin
  z.orders.create(items: items)
rescue Zatabox::Error => e
  e.code        # "TICKET_SOLD_OUT"
  e.status      # 409 (0 for network errors)
  e.message     # human-readable
  e.request_id  # "req_01J9..."
  e.details     # { "ticketTypeId" => "tkt_8f2k" }
end
```

### Common error codes

| `code` | `status` | Meaning |
| --- | --- | --- |
| `VALIDATION_ERROR` | 400 | Body failed validation. |
| `UNAUTHORIZED` / `INVALID_TOKEN` | 401 | Missing/expired credential. |
| `WRONG_ENV` | 403 | Test key on production or vice-versa. |
| `INSUFFICIENT_SCOPE` | 403 | Key lacks the route's scope. |
| `NOT_FOUND` | 404 | No such resource. |
| `CONFLICT` / `IDEMPOTENCY_KEY_REUSED` | 409 | Unique/idempotency clash. |
| `TICKET_SOLD_OUT` | 409 | Inventory exhausted. |
| `RATE_LIMITED` | 429 | Throttled; see `details["retryAfter"]`. |
| `INTERNAL_ERROR` | 500 | Server error (auto-retried). |
| `NETWORK_ERROR` | 0 | Connection/timeout after retries (SDK-side). |
| `MISSING_SIGNATURE` / `INVALID_SIGNATURE` / `SIGNATURE_EXPIRED` | | webhook verify failures. |

## Idempotency

Every write auto-sends an `Idempotency-Key` (fresh UUID). The server caches the result
for 24h replaying the same key + body returns the original; the same key with a
different body returns `409 IDEMPOTENCY_KEY_REUSED`. Pass your own to make a retry safe:

```ruby
require "securerandom"
key = SecureRandom.uuid
z.orders.create({ items: items }, idempotency_key: key)
z.orders.create({ items: items }, idempotency_key: key)  # safe retry no double charge
```

## Retries, timeouts & networking

- **Timeouts** open/read bounded by `timeout` (seconds); treated as retryable.
- **Retries** `5xx`/network/timeout retried up to `max_retries` with exponential
  backoff; `4xx` never retried.
- **Rate limits** `429` raises `Zatabox::Error` with `code "RATE_LIMITED"` and
  `details["retryAfter"]`.

## Pagination

```ruby
z.paginate(z.events.method(:list), q: "jazz", limit: 50) do |page|
  page["items"].each { |ev| puts ev["id"] }
end
```

`paginate(callable, query)` follows the cursor across both response shapes. Pass a
callable for any cursor-paginated list (e.g. `z.events.method(:list)`,
`z.users.method(:tickets)`). Without a block it returns an `Enumerator`. To page
manually:

```ruby
cursor = nil
loop do
  page = z.events.list(limit: 50, cursor: cursor)
  # ...
  cursor = page.dig("pagination", "cursor") || page["nextCursor"]
  break unless cursor
end
```

## Live check-in stream (SSE)

`z.checkin.live_url(event_id)` returns the stream URL; consume it with your preferred
SSE client (passing `Authorization: Bearer <key>`).

## Verifying inbound webhooks

Signature header: `X-Zatabox-Signature: t=<unix>,v1=<hex-hmac-sha256>`; the signed
payload is `<t>.<raw_body>`, HMAC-SHA256 with your endpoint secret (constant-time, 5-min
tolerance). **Verify the raw bytes, not a re-serialized object.**

```ruby
# Rails / Rack
def zatabox_webhook
  event = $zatabox.webhooks.verify(
    request.raw_post,
    request.headers["X-Zatabox-Signature"],
    ENV.fetch("ZATABOX_WEBHOOK_SECRET")
  )
  fulfil(event) if event["type"] == "order.paid"
  head :ok
rescue Zatabox::Error
  head :bad_request
end
```

## End-to-end recipes

```ruby
# Sell a ticket (guest checkout)
order = z.orders.create(items: [{ ticketTypeId: tt, quantity: 2 }], guestEmail: "fan@example.com")
intent = z.orders.pay(order["id"], provider: "paystack")
paid = z.payments.verify(orderId: order["id"])

# Check in at the gate
res = z.checkin.scan(qrData: qr, gateName: "Main", deviceId: device)
stats = z.checkin.stats(event_id)
```

## Thread safety

A `Client` holds only its credential and configuration; each call opens its own
`Net::HTTP` connection, so a client is safe to share across threads. Create one per
credential and reuse it.

## Troubleshooting & FAQ

- **`ArgumentError: pass api_key or bearer_token`** provide a credential.
- **`403 WRONG_ENV`** match the key prefix to the environment.
- **`409 IDEMPOTENCY_KEY_REUSED`** reused a key with a different body.
- **Webhook `INVALID_SIGNATURE`** verify `request.raw_post`, not parsed params.
- **A list looks short** iterate with `z.paginate(...)`.

## Versioning & support

SemVer; version is `Zatabox::VERSION`. API base `https://api.zatabox.com/api/v1` ·
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
| `register(body = nil, opts = {})` | `POST /api/v1/auth/register` | Register an account; returns the user plus an accessToken/refreshToken pair. |
| `login(body = nil, opts = {})` | `POST /api/v1/auth/login` | Log in with email + password; returns a JWT pair (or a 2FA challenge). |
| `login_verify2fa(body = nil, opts = {})` | `POST /api/v1/auth/2fa-verify` | Complete a 2FA login challenge; returns the JWT pair. |
| `request_token(body = nil, opts = {})` | `POST /api/v1/auth/token/request` | Passwordless: email a buyer a 6-digit login code. |
| `exchange_token(body = nil, opts = {})` | `POST /api/v1/auth/token/exchange` | Passwordless: exchange email + 6-digit code for a JWT pair. |
| `refresh(body = nil, opts = {})` | `POST /api/v1/auth/refresh` | Refresh an expired access token (rotates the refresh token). |
| `logout(body = nil, opts = {})` | `POST /api/v1/auth/logout` | Revoke a refresh token. |

### `client.events` Events (Public)

Public event discovery and read, plus external ticket issuance.

| Method | Endpoint | Description |
| --- | --- | --- |
| `list(query = nil, opts = {})` | `GET /api/v1/events` | List and search published public events (cursor-paginated). |
| `get(slug, query = nil, opts = {})` | `GET /api/v1/events/:slug` | Event detail by slug (organizer info, schedule, active ticket types). |
| `tickets(id, query = nil, opts = {})` | `GET /api/v1/events/:id/tickets` | List an event's ticket types with live availability. |
| `issue(event_id, body = nil, opts = {})` | `POST /api/v1/events/:eventId/issue` | Issue tickets you sold elsewhere (developer-handled payment; 3% wallet fee on paid tickets). |

### `client.organizer` Organizer

Organizer surface: organization read, events, ticket types, schedule sessions, seating sections and promo codes.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get_organization(id, query = nil, opts = {})` | `GET /api/v1/organizer/organizations/:id` | Get organization details and per-currency wallet balances. |
| `create_event(body = nil, opts = {})` | `POST /api/v1/organizer/events` | Create a draft event. |
| `update_event(id, body = nil, opts = {})` | `PUT /api/v1/organizer/events/:id` | Partial-update an event. |
| `publish_event(id, body = nil, opts = {})` | `POST /api/v1/organizer/events/:id/publish` | Publish a draft event. |
| `unpublish_event(id, body = nil, opts = {})` | `POST /api/v1/organizer/events/:id/unpublish` | Unpublish a published event back to draft. |
| `delete_event(id, body = nil, opts = {})` | `DELETE /api/v1/organizer/events/:id` | Cancel an event. |
| `create_ticket(id, body = nil, opts = {})` | `POST /api/v1/organizer/events/:id/tickets` | Create a ticket type. |
| `schedule(id, query = nil, opts = {})` | `GET /api/v1/organizer/events/:id/schedule` | List schedule sessions (running order). |
| `create_schedule(id, body = nil, opts = {})` | `POST /api/v1/organizer/events/:id/schedule` | Add a schedule session. |
| `update_schedule(id, session_id, body = nil, opts = {})` | `PUT /api/v1/organizer/events/:id/schedule/:sessionId` | Update a schedule session. |
| `delete_schedule(id, session_id, body = nil, opts = {})` | `DELETE /api/v1/organizer/events/:id/schedule/:sessionId` | Delete a schedule session. |
| `sections(id, query = nil, opts = {})` | `GET /api/v1/organizer/events/:id/sections` | List seating/capacity sections. |
| `create_section(id, body = nil, opts = {})` | `POST /api/v1/organizer/events/:id/sections` | Add a seating section. |
| `update_section(id, section_id, body = nil, opts = {})` | `PUT /api/v1/organizer/events/:id/sections/:sectionId` | Update a seating section. |
| `delete_section(id, section_id, body = nil, opts = {})` | `DELETE /api/v1/organizer/events/:id/sections/:sectionId` | Delete a seating section. |
| `promo_codes(query = nil, opts = {})` | `GET /api/v1/organizer/promo-codes` | List promo codes (optionally filtered by event). |
| `create_promo_code(body = nil, opts = {})` | `POST /api/v1/organizer/promo-codes` | Create a promo code. |
| `update_promo_code(id, body = nil, opts = {})` | `PUT /api/v1/organizer/promo-codes/:id` | Update a promo code. |
| `delete_promo_code(id, body = nil, opts = {})` | `DELETE /api/v1/organizer/promo-codes/:id` | Delete or disable a promo code. |

### `client.event_customization` Event page customization

Per-event public-page theming and the “Good to know” FAQ.

| Method | Endpoint | Description |
| --- | --- | --- |
| `get(id, query = nil, opts = {})` | `GET /api/v1/organizer/event-customization/:id` | Get an event's page customization (theme, layout, FAQ, SEO). |
| `update(id, body = nil, opts = {})` | `PUT /api/v1/organizer/event-customization/:id` | Update an event's page customization (incl. the FAQ list). |

### `client.tickets` Tickets

Checkout-time ticket helpers.

| Method | Endpoint | Description |
| --- | --- | --- |
| `validate_promo(body = nil, opts = {})` | `POST /api/v1/tickets/promo/validate` | Validate a promo code against a cart (read-only preview, does not consume a use). |

### `client.orders` Orders

Carted checkout: create, read, pay, cancel.

| Method | Endpoint | Description |
| --- | --- | --- |
| `create(body = nil, opts = {})` | `POST /api/v1/orders` | Create an order (guest checkout needs only name + email). |
| `get(id, query = nil, opts = {})` | `GET /api/v1/orders/:id` | Get an order (pass ?token for guest reads). |
| `pay(id, body = nil, opts = {})` | `POST /api/v1/orders/:id/pay` | Initiate payment (provider: nowpayments \| paystack \| flutterwave). |
| `cancel(id, body = nil, opts = {})` | `POST /api/v1/orders/:id/cancel` | Cancel an unpaid order and release held inventory. |

### `client.payments` Payments

Verify charges, read payment status, list crypto coins.

| Method | Endpoint | Description |
| --- | --- | --- |
| `verify(body = nil, opts = {})` | `POST /api/v1/payments/verify` | Actively verify a payment with the provider and issue tickets (idempotent, poll-safe). |
| `get(order_id, query = nil, opts = {})` | `GET /api/v1/payments/:orderId` | Read payment/order status and attempts (read-only). |
| `crypto_currencies(query = nil, opts = {})` | `GET /api/v1/payments/crypto/currencies` | List supported NOWPayments crypto coins (for the payCurrency value). |

### `client.checkin` Check-in

Gate scanning, offline manifests + sync, live stats.

| Method | Endpoint | Description |
| --- | --- | --- |
| `scan(body = nil, opts = {})` | `POST /api/v1/checkin/scan` | Validate a QR, barcode or 6-character door code at the gate. |
| `manual(id, body = nil, opts = {})` | `POST /api/v1/checkin/event/:id/manual` | Manually check in a typed ticket code. |
| `manifest(id, query = nil, opts = {})` | `GET /api/v1/checkin/event/:id/manifest` | Hashed guest-list manifest for offline scanning (pass ?since for a delta). |
| `batch(body = nil, opts = {})` | `POST /api/v1/checkin/batch` | Sync up to 500 queued offline scans. |
| `stats(id, query = nil, opts = {})` | `GET /api/v1/checkin/event/:id/stats` | Check-in totals, capacity %, entry rate and per-gate breakdown. |
| `gate(id, gate, query = nil, opts = {})` | `GET /api/v1/checkin/event/:id/gate/:gate` | Per-gate check-in stats slice. |
| `live_url(id, query = nil)` | `GET /api/v1/checkin/event/:id/live` | Server-Sent Events stream a stats snapshot every 2 seconds. |

### `client.community` Community

Verified-attendee reviews, organizer follows/subscribers and event waitlists.

| Method | Endpoint | Description |
| --- | --- | --- |
| `submit_review(body = nil, opts = {})` | `POST /api/v1/community/reviews` | Review an event (checked-in ticket holders only; ticketCode + email prove attendance). |
| `follow(org_id, body = nil, opts = {})` | `POST /api/v1/community/orgs/:orgId/follow` | Follow an organizer (subscribe to new-event announcements). |
| `followers(org_id, query = nil, opts = {})` | `GET /api/v1/community/orgs/:orgId/followers` | List an organizer's subscribers (organizer auth). |
| `remove_follower(org_id, follower_id, body = nil, opts = {})` | `DELETE /api/v1/community/orgs/:orgId/followers/:followerId` | Remove a subscriber (organizer auth). |
| `join_waitlist(event_id, body = nil, opts = {})` | `POST /api/v1/community/events/:eventId/waitlist` | Join an event waitlist (offers fire on cancellations). |

### `client.growth` Growth (Organizer)

Comp tickets, CSV import, broadcasts and attendee tags.

| Method | Endpoint | Description |
| --- | --- | --- |
| `mint_comps(event_id, body = nil, opts = {})` | `POST /api/v1/organizer/growth/events/:eventId/comps` | Bulk-mint and email complimentary tickets. |
| `import_comps_csv(event_id, body = nil, opts = {})` | `POST /api/v1/organizer/growth/events/:eventId/comps/import-csv` | Import attendees (comp tickets) from CSV. |
| `broadcast_event(event_id, body = nil, opts = {})` | `POST /api/v1/organizer/growth/events/:eventId/broadcast` | Email a broadcast to an event's attendees (replies thread to the organizer inbox). |
| `add_tags(body = nil, opts = {})` | `POST /api/v1/organizer/growth/tags` | Tag attendees (additive; powers broadcast filters and CRM segments). |
| `remove_tag(body = nil, opts = {})` | `DELETE /api/v1/organizer/growth/tags` | Remove an attendee tag. |

### `client.users` Buyers

The authenticated buyer: profile, ticket wallet, data export, refunds, reports and organizer messaging.

| Method | Endpoint | Description |
| --- | --- | --- |
| `me(query = nil, opts = {})` | `GET /api/v1/users/me` | Current buyer profile. |
| `tickets(query = nil, opts = {})` | `GET /api/v1/users/me/tickets` | The buyer's ticket wallet (cursor-paginated). |
| `export(query = nil, opts = {})` | `GET /api/v1/users/me/export` | GDPR data export one JSON download of everything on the account. |
| `create_refund(body = nil, opts = {})` | `POST /api/v1/users/me/refunds` | Request a refund for a ticket. |
| `create_report(body = nil, opts = {})` | `POST /api/v1/users/me/reports` | File a report against an event or organizer. |
| `messages(query = nil, opts = {})` | `GET /api/v1/users/me/messages` | The buyer's message threads with organizers. |
| `send_ticket_message(ticket_id, body = nil, opts = {})` | `POST /api/v1/users/me/tickets/:ticketId/message` | Message the organizer about a ticket (rate-limited). |

### `client.integrations` API keys

Manage your organization's own API keys (organizer owner/admin auth).

| Method | Endpoint | Description |
| --- | --- | --- |
| `create_api_key(org_id, body = nil, opts = {})` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys` | Create an API key (plaintext secret returned exactly once). |
| `list_api_keys(org_id, query = nil, opts = {})` | `GET /api/v1/organizer/integrations/org/:orgId/api-keys` | List API keys (prefixes and metadata only, never the secret). |
| `update_api_key(org_id, key_id, body = nil, opts = {})` | `PUT /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Update a key (rename, pause, re-scope). |
| `rotate_api_key(org_id, key_id, body = nil, opts = {})` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId/rotate` | Rotate a key's secret (new secret returned once; old one invalidated). |
| `delete_api_key(org_id, key_id, body = nil, opts = {})` | `DELETE /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Revoke an API key. |

### `client.webhooks` Webhooks

Register webhook endpoints, manage secrets, inspect and replay deliveries. (Use webhooks.verify() to validate inbound signatures.)

| Method | Endpoint | Description |
| --- | --- | --- |
| `create(body = nil, opts = {})` | `POST /api/v1/webhooks` | Create a webhook endpoint (signing secret returned exactly once). |
| `list(query = nil, opts = {})` | `GET /api/v1/webhooks` | List webhook endpoints. |
| `update(id, body = nil, opts = {})` | `PUT /api/v1/webhooks/:id` | Update a webhook endpoint. |
| `delete(id, body = nil, opts = {})` | `DELETE /api/v1/webhooks/:id` | Delete a webhook endpoint. |
| `test(id, body = nil, opts = {})` | `POST /api/v1/webhooks/:id/test` | Send a signed test event to the endpoint. |
| `rotate_secret(id, body = nil, opts = {})` | `POST /api/v1/webhooks/:id/rotate-secret` | Rotate the signing secret (new secret returned once). |
| `deliveries(id, query = nil, opts = {})` | `GET /api/v1/webhooks/:id/deliveries` | List delivery attempts for an endpoint. |
| `replay(id, body = nil, opts = {})` | `POST /api/v1/webhooks/deliveries/:id/replay` | Replay a past delivery. |
| `catalog(query = nil, opts = {})` | `GET /api/v1/webhooks/catalog` | List every subscribable event type (no auth). |

<!-- END ENDPOINTS -->
