# zatabox-go Go SDK

Official **Go SDK** for the [Zatabox Tickets](https://zatabox.com) REST API the
white-label event-ticketing platform. A small, dependency-free client over
`https://api.zatabox.com/api/v1` that handles auth, sandbox routing, idempotency,
retries, pagination, live (SSE) streaming and webhook verification.

- **Zero dependencies** standard library only (`net/http`, `crypto/hmac`).
- **Complete** every one of the **78 REST endpoints** is a method.
- **Generated, never drifts** emitted from the canonical
  [`endpoints.json`](../spec/endpoints.json) spec.
- **Go 1.18+**.

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
- [Responses & decoding](#responses--decoding)
- [Error handling](#error-handling)
- [Idempotency](#idempotency)
- [Retries, timeouts & context](#retries-timeouts--context)
- [Pagination](#pagination)
- [Live check-in stream (SSE)](#live-check-in-stream-sse)
- [Verifying inbound webhooks](#verifying-inbound-webhooks)
- [End-to-end recipes](#end-to-end-recipes)
- [Concurrency](#concurrency)
- [Troubleshooting & FAQ](#troubleshooting--faq)
- [Full endpoint reference](#full-endpoint-reference)
- [Versioning & support](#versioning--support)
- [License](#license)

---

## Requirements

- **Go 1.18 or newer**. No third-party modules.
- A Zatabox **API key** (`vt_live_…` / `vt_test_…`), a **portal JWT**, or an **MCP
  token**. Mint API keys in the organizer portal → Integrations, or the
  [sandbox console](https://tester.zatabox.com).

## Installation

This SDK is **distributed via GitHub** the Go module lives in the `go/` directory of
[`mysmartrobot/zataboxtickets`](https://github.com/mysmartrobot/zataboxtickets), so its
module path is the repo path plus `/go`:

```bash
go get github.com/mysmartrobot/zataboxtickets/go@latest
```

Pin to a commit or a `go/`-prefixed tag for reproducible builds, e.g.
`go get github.com/mysmartrobot/zataboxtickets/go@<commit-sha>`. The module has **zero
dependencies**. Import it (the package is named `zatabox`):

```go
import zatabox "github.com/mysmartrobot/zataboxtickets/go"
```

> Offline / private-network? `git clone` the repo and add a
> `replace github.com/mysmartrobot/zataboxtickets/go => ../zataboxtickets/go` directive
> to your `go.mod`.

## Quick start

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	zatabox "github.com/mysmartrobot/zataboxtickets/go"
)

func main() {
	ctx := context.Background()
	// A vt_test_ key auto-routes to the sandbox; a vt_live_ key to production.
	z := zatabox.New(os.Getenv("ZATABOX_API_KEY"))

	data, err := z.Organizer.CreateEvent(ctx, zatabox.WithBody(map[string]interface{}{
		"title": "Warehouse Sessions 004", "category": "music",
		"startDate": "2026-08-22T20:00:00Z", "endDate": "2026-08-23T02:00:00Z",
		"timezone": "Africa/Lagos", "venueType": "physical", "venueCity": "Lagos",
		"capacity": 450,
	}))
	if err != nil {
		panic(err)
	}
	var event struct{ ID, Slug string }
	_ = json.Unmarshal(data, &event)

	_, _ = z.Organizer.CreateTicket(ctx, event.ID, zatabox.WithBody(map[string]interface{}{
		"name": "General Admission", "type": "general", "price": 5000, "currency": "NGN",
		"quantityTotal": 450, "saleStart": "2026-07-01T00:00:00Z", "saleEnd": "2026-08-22T20:00:00Z",
	}))
	_, _ = z.Organizer.PublishEvent(ctx, event.ID)
	fmt.Println("published", event.ID)
}
```

## Authentication

The SDK forwards one `Authorization: Bearer <token>` header. Three ways to authenticate:

```go
zatabox.New("vt_live_...")                                  // scoped API key (prefix selects env)
zatabox.New("vt_test_...")                                  // sandbox (auto-routed)
zatabox.New("", zatabox.WithBearerToken("eyJ..."))          // portal JWT or vt_mcp_ token
zatabox.New("vt_test_...", zatabox.WithBaseURL("http://localhost:4100"))
```

### Passwordless buyer login

```go
anon := zatabox.New("", zatabox.WithBearerToken("unused"), zatabox.WithBaseURL("https://api.zatabox.com"))
_, _ = anon.Auth.RequestToken(ctx, zatabox.WithBody(map[string]interface{}{"email": "fan@example.com"}))
sess, _ := anon.Auth.ExchangeToken(ctx, zatabox.WithBody(map[string]interface{}{"email": "fan@example.com", "code": "123456"}))

var s struct{ AccessToken string `json:"accessToken"` }
_ = json.Unmarshal(sess, &s)
buyer := zatabox.New("", zatabox.WithBearerToken(s.AccessToken))
```

### Refreshing & swapping tokens

```go
next, _ := buyer.Auth.Refresh(ctx, zatabox.WithBody(map[string]interface{}{"refreshToken": refresh}))
var n struct{ AccessToken string `json:"accessToken"` }
_ = json.Unmarshal(next, &n)
buyer.SetBearerToken(n.AccessToken)
```

### API-key scopes

Keys can be minted with least-privilege scopes: `events:read`, `events:write`,
`tickets:read`, `tickets:write`, `orders:read`, `orders:write`, `attendees:read`,
`attendees:write`, `checkin:write`, `payouts:read`, `payouts:write`, `webhooks:manage`,
`analytics:read`, `*`. A call beyond a key's scopes returns `403 INSUFFICIENT_SCOPE`.

## Sandbox / test mode

`vt_test_` keys auto-route to the Zatabox **sandbox** at `https://sandbox.zatabox.com`
a full mirror of the API with no real charges, emails or SMS.

```go
zatabox.New("vt_test_...")   // → sandbox.zatabox.com
zatabox.New("vt_live_...")   // → api.zatabox.com
zatabox.New("vt_test_...", zatabox.WithBaseURL("http://localhost:4100"))
```

Mint/rotate `vt_test_` keys, watch **live request logs**, see usage and browse the
endpoint catalog in the **sandbox console at https://tester.zatabox.com** (sign in
with your production account). A test key used against production or vice-versa 
returns `403 WRONG_ENV`.

## Client configuration

`New(apiKey, opts...)` plus functional options:

```go
z := zatabox.New("vt_live_...",
	zatabox.WithBearerToken("eyJ..."),                 // alternative credential
	zatabox.WithBaseURL("https://api.zatabox.com"),    // explicit host
	zatabox.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}),
	zatabox.WithMaxRetries(3),                         // 5xx / network / timeout retries
	zatabox.WithUserAgent("my-app/1.0"),
)
```

| Option | Default | Description |
| --- | --- | --- |
| `New(apiKey, …)` | | API key positional; `""` + `WithBearerToken` for JWT/MCP. |
| `WithBearerToken(t)` | | Portal JWT / `vt_mcp_…` token. |
| `WithBaseURL(u)` | resolved from key | Explicit API origin; wins over prefix routing. |
| `WithHTTPClient(h)` | `&http.Client{Timeout: 30s}` | Inject a custom client/transport. |
| `WithMaxRetries(n)` | `2` | Retries for `5xx`/network/timeout (never `4xx`). |
| `WithUserAgent(s)` | `zatabox-go/<version>` | Overrides the `User-Agent` header. |

## How methods map to endpoints

`client.<Namespace>.<Method>(ctx, …pathParams, …opts)`. Namespaces are exported fields:

`Auth`, `Events`, `Organizer`, `EventCustomization`, `Tickets`, `Orders`, `Payments`,
`Checkin`, `Community`, `Growth`, `Users`, `Integrations`, `Webhooks`.

Every method takes a `context.Context`, then any path params (strings), then variadic
`RequestOption`s. Bodies and query params are passed as options:

```go
z.Events.List(ctx, zatabox.WithQuery(map[string]interface{}{"q": "jazz", "limit": 20}))
z.Events.Get(ctx, "warehouse-sessions-004")
z.Organizer.UpdateSchedule(ctx, eventID, sessionID, zatabox.WithBody(map[string]interface{}{"sessionTitle": "Keynote"}))
z.Orders.Create(ctx, zatabox.WithBody(cart), zatabox.WithIdempotencyKey(myUUID))
```

Options: `WithBody(v)`, `WithQuery(map)`, `WithIdempotencyKey(s)`, `WithHeader(k, v)`.

## Responses & decoding

JSON methods return `json.RawMessage` (the unwrapped `data` document) so you can decode
into your own structs:

```go
data, err := z.Events.Get(ctx, slug)
var ev struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}
_ = json.Unmarshal(data, &ev)
```

## Error handling

Any non-2xx (and network/timeout/webhook failures) returns a `*zatabox.Error`:

```go
data, err := z.Orders.Create(ctx, zatabox.WithBody(cart))
if err != nil {
	var e *zatabox.Error
	if errors.As(err, &e) {
		e.Code        // "TICKET_SOLD_OUT"
		e.Status      // 409 (0 for network errors)
		e.RequestID   // "req_01J9..."
		e.Details     // json.RawMessage
		e.RetryAfter  // set on 429s
	}
	return err
}
```

### Common error codes

| `Code` | `Status` | Meaning |
| --- | --- | --- |
| `VALIDATION_ERROR` | 400 | Body failed validation. |
| `UNAUTHORIZED` / `INVALID_TOKEN` | 401 | Missing/expired credential. |
| `WRONG_ENV` | 403 | Test key on production or vice-versa. |
| `INSUFFICIENT_SCOPE` | 403 | Key lacks the route's scope. |
| `NOT_FOUND` | 404 | No such resource. |
| `CONFLICT` / `IDEMPOTENCY_KEY_REUSED` | 409 | Unique/idempotency clash. |
| `TICKET_SOLD_OUT` | 409 | Inventory exhausted. |
| `RATE_LIMITED` | 429 | Throttled; see `e.RetryAfter`. |
| `INTERNAL_ERROR` | 500 | Server error (auto-retried). |
| `NETWORK_ERROR` | 0 | Connection/timeout after retries (SDK-side). |
| `MISSING_SIGNATURE` / `INVALID_SIGNATURE` / `SIGNATURE_EXPIRED` | | webhook verify failures. |

## Idempotency

Every write auto-sends an `Idempotency-Key` (fresh UUIDv4). The server caches the result
for 24h replaying the same key + body returns the original; the same key with a
different body returns `409 IDEMPOTENCY_KEY_REUSED`. Pass your own to make a retry safe:

```go
key := "..." // a stable UUID
z.Orders.Create(ctx, zatabox.WithBody(cart), zatabox.WithIdempotencyKey(key))
z.Orders.Create(ctx, zatabox.WithBody(cart), zatabox.WithIdempotencyKey(key)) // safe retry
```

## Retries, timeouts & context

- **Context** every call takes a `context.Context`; cancel/deadline propagate to the
  HTTP request.
- **Timeouts** set via the `*http.Client` (default 30s) or your context deadline.
- **Retries** `5xx`/network/timeout retried up to `WithMaxRetries` with exponential
  backoff; `4xx` never retried.
- **Rate limits** `429` returns `*Error` with `Code == "RATE_LIMITED"` and
  `RetryAfter` set.

## Pagination

`Paginate` calls a list method repeatedly, following the cursor across both response
shapes, invoking your callback per page:

```go
err := z.Paginate(ctx, z.Events.List, map[string]interface{}{"q": "jazz", "limit": 50}, func(page json.RawMessage) error {
	var p struct{ Items []json.RawMessage `json:"items"` }
	_ = json.Unmarshal(page, &p)
	// ... handle p.Items
	return nil
})
```

Any cursor-paginated list method matching `func(ctx, ...RequestOption)
(json.RawMessage, error)` e.g. `z.Events.List`, `z.Users.Tickets`,
`z.Webhooks.Deliveries` can be passed.

## Live check-in stream (SSE)

`z.Checkin.LiveURL(eventID)` returns the stream URL; consume it with your SSE client
(passing `Authorization: Bearer <key>`).

## Verifying inbound webhooks

Signature header: `X-Zatabox-Signature: t=<unix>,v1=<hex-hmac-sha256>`; the signed
payload is `<t>.<rawBody>`, HMAC-SHA256 with your endpoint secret (constant-time, 5-min
tolerance). **Verify the raw request body.**

```go
func handler(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	event, err := z.Webhooks.Verify(raw, r.Header.Get("X-Zatabox-Signature"), os.Getenv("ZATABOX_WEBHOOK_SECRET"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var e struct{ Type string `json:"type"` }
	_ = json.Unmarshal(event, &e)
	// switch e.Type { case "order.paid": ... }
	w.WriteHeader(http.StatusOK)
}
```

## End-to-end recipes

```go
// Sell a ticket (guest checkout)
order, _ := z.Orders.Create(ctx, zatabox.WithBody(map[string]interface{}{
	"items": []map[string]interface{}{{"ticketTypeId": tt, "quantity": 2}},
	"guestEmail": "fan@example.com",
}))
var o struct{ ID string `json:"id"` }
_ = json.Unmarshal(order, &o)
_, _ = z.Orders.Pay(ctx, o.ID, zatabox.WithBody(map[string]interface{}{"provider": "paystack"}))
_, _ = z.Payments.Verify(ctx, zatabox.WithBody(map[string]interface{}{"orderId": o.ID}))

// Check in at the gate
_, _ = z.Checkin.Scan(ctx, zatabox.WithBody(map[string]interface{}{"qrData": qr, "gateName": "Main"}))
_, _ = z.Checkin.Stats(ctx, eventID)
```

## Concurrency

A `*Client` is safe for concurrent use by multiple goroutines (it wraps a single
`*http.Client`). Construct one per credential and share it.

## Troubleshooting & FAQ

- **`403 WRONG_ENV`** match the key prefix to the environment.
- **`409 IDEMPOTENCY_KEY_REUSED`** reused a key with a different body.
- **Webhook `INVALID_SIGNATURE`** verify the raw `r.Body` bytes, not a decoded struct.
- **A list looks short** iterate with `z.Paginate(...)`.
- **Unauthenticated 401s** you called `New("")` without `WithBearerToken`; supply a
  credential.

## Versioning & support

SemVer; version is the `zatabox.Version` constant. API base
`https://api.zatabox.com/api/v1` · Docs <https://zatabox.com/docs> ·
developers@zatabox.com.

## License

MIT

---

## Full endpoint reference

<!-- BEGIN ENDPOINTS (generated by scripts/generate.mjs do not edit) -->

The SDK exposes **78 endpoints** across **13 namespaces**. Every method is listed below with its idiomatic signature, the underlying HTTP route, and what it does. Path parameters are positional; reads take an optional query map and writes take an optional body, both followed by a call-options bag.

### `client.Auth` Auth

Account registration, password + passwordless sign-in, 2FA login and token refresh.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Register(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/register` | Register an account; returns the user plus an accessToken/refreshToken pair. |
| `Login(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/login` | Log in with email + password; returns a JWT pair (or a 2FA challenge). |
| `LoginVerify2fa(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/2fa-verify` | Complete a 2FA login challenge; returns the JWT pair. |
| `RequestToken(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/token/request` | Passwordless: email a buyer a 6-digit login code. |
| `ExchangeToken(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/token/exchange` | Passwordless: exchange email + 6-digit code for a JWT pair. |
| `Refresh(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/refresh` | Refresh an expired access token (rotates the refresh token). |
| `Logout(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/logout` | Revoke a refresh token. |

### `client.Events` Events (Public)

Public event discovery and read, plus external ticket issuance.

| Method | Endpoint | Description |
| --- | --- | --- |
| `List(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/events` | List and search published public events (cursor-paginated). |
| `Get(ctx, slug, opts...) (json.RawMessage, error)` | `GET /api/v1/events/:slug` | Event detail by slug (organizer info, schedule, active ticket types). |
| `Tickets(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/events/:id/tickets` | List an event's ticket types with live availability. |
| `Issue(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/events/:eventId/issue` | Issue tickets you sold elsewhere (developer-handled payment; 3% wallet fee on paid tickets). |

### `client.Organizer` Organizer

Organizer surface: organization read, events, ticket types, schedule sessions, seating sections and promo codes.

| Method | Endpoint | Description |
| --- | --- | --- |
| `GetOrganization(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/organizations/:id` | Get organization details and per-currency wallet balances. |
| `CreateEvent(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events` | Create a draft event. |
| `UpdateEvent(ctx, id, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/events/:id` | Partial-update an event. |
| `PublishEvent(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events/:id/publish` | Publish a draft event. |
| `UnpublishEvent(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events/:id/unpublish` | Unpublish a published event back to draft. |
| `DeleteEvent(ctx, id, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/events/:id` | Cancel an event. |
| `CreateTicket(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events/:id/tickets` | Create a ticket type. |
| `Schedule(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/events/:id/schedule` | List schedule sessions (running order). |
| `CreateSchedule(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events/:id/schedule` | Add a schedule session. |
| `UpdateSchedule(ctx, id, sessionId, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/events/:id/schedule/:sessionId` | Update a schedule session. |
| `DeleteSchedule(ctx, id, sessionId, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/events/:id/schedule/:sessionId` | Delete a schedule session. |
| `Sections(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/events/:id/sections` | List seating/capacity sections. |
| `CreateSection(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events/:id/sections` | Add a seating section. |
| `UpdateSection(ctx, id, sectionId, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/events/:id/sections/:sectionId` | Update a seating section. |
| `DeleteSection(ctx, id, sectionId, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/events/:id/sections/:sectionId` | Delete a seating section. |
| `PromoCodes(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/promo-codes` | List promo codes (optionally filtered by event). |
| `CreatePromoCode(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/promo-codes` | Create a promo code. |
| `UpdatePromoCode(ctx, id, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/promo-codes/:id` | Update a promo code. |
| `DeletePromoCode(ctx, id, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/promo-codes/:id` | Delete or disable a promo code. |

### `client.EventCustomization` Event page customization

Per-event public-page theming and the “Good to know” FAQ.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Get(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/event-customization/:id` | Get an event's page customization (theme, layout, FAQ, SEO). |
| `Update(ctx, id, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/event-customization/:id` | Update an event's page customization (incl. the FAQ list). |

### `client.Tickets` Tickets

Checkout-time ticket helpers.

| Method | Endpoint | Description |
| --- | --- | --- |
| `ValidatePromo(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/tickets/promo/validate` | Validate a promo code against a cart (read-only preview, does not consume a use). |

### `client.Orders` Orders

Carted checkout: create, read, pay, cancel.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Create(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/orders` | Create an order (guest checkout needs only name + email). |
| `Get(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/orders/:id` | Get an order (pass ?token for guest reads). |
| `Pay(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/orders/:id/pay` | Initiate payment (provider: nowpayments \| paystack \| flutterwave). |
| `Cancel(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/orders/:id/cancel` | Cancel an unpaid order and release held inventory. |

### `client.Payments` Payments

Verify charges, read payment status, list crypto coins.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Verify(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/payments/verify` | Actively verify a payment with the provider and issue tickets (idempotent, poll-safe). |
| `Get(ctx, orderId, opts...) (json.RawMessage, error)` | `GET /api/v1/payments/:orderId` | Read payment/order status and attempts (read-only). |
| `CryptoCurrencies(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/payments/crypto/currencies` | List supported NOWPayments crypto coins (for the payCurrency value). |

### `client.Checkin` Check-in

Gate scanning, offline manifests + sync, live stats.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Scan(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/checkin/scan` | Validate a QR, barcode or 6-character door code at the gate. |
| `Manual(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/checkin/event/:id/manual` | Manually check in a typed ticket code. |
| `Manifest(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/checkin/event/:id/manifest` | Hashed guest-list manifest for offline scanning (pass ?since for a delta). |
| `Batch(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/checkin/batch` | Sync up to 500 queued offline scans. |
| `Stats(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/checkin/event/:id/stats` | Check-in totals, capacity %, entry rate and per-gate breakdown. |
| `Gate(ctx, id, gate, opts...) (json.RawMessage, error)` | `GET /api/v1/checkin/event/:id/gate/:gate` | Per-gate check-in stats slice. |
| `LiveURL(id, query...) string` | `GET /api/v1/checkin/event/:id/live` | Server-Sent Events stream a stats snapshot every 2 seconds. |

### `client.Community` Community

Verified-attendee reviews, organizer follows/subscribers and event waitlists.

| Method | Endpoint | Description |
| --- | --- | --- |
| `SubmitReview(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/community/reviews` | Review an event (checked-in ticket holders only; ticketCode + email prove attendance). |
| `Follow(ctx, orgId, opts...) (json.RawMessage, error)` | `POST /api/v1/community/orgs/:orgId/follow` | Follow an organizer (subscribe to new-event announcements). |
| `Followers(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/community/orgs/:orgId/followers` | List an organizer's subscribers (organizer auth). |
| `RemoveFollower(ctx, orgId, followerId, opts...) (json.RawMessage, error)` | `DELETE /api/v1/community/orgs/:orgId/followers/:followerId` | Remove a subscriber (organizer auth). |
| `JoinWaitlist(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/community/events/:eventId/waitlist` | Join an event waitlist (offers fire on cancellations). |

### `client.Growth` Growth (Organizer)

Comp tickets, CSV import, broadcasts and attendee tags.

| Method | Endpoint | Description |
| --- | --- | --- |
| `MintComps(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/growth/events/:eventId/comps` | Bulk-mint and email complimentary tickets. |
| `ImportCompsCsv(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/growth/events/:eventId/comps/import-csv` | Import attendees (comp tickets) from CSV. |
| `BroadcastEvent(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/growth/events/:eventId/broadcast` | Email a broadcast to an event's attendees (replies thread to the organizer inbox). |
| `AddTags(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/growth/tags` | Tag attendees (additive; powers broadcast filters and CRM segments). |
| `RemoveTag(ctx, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/growth/tags` | Remove an attendee tag. |

### `client.Users` Buyers

The authenticated buyer: profile, ticket wallet, data export, refunds, reports and organizer messaging.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Me(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me` | Current buyer profile. |
| `Tickets(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/tickets` | The buyer's ticket wallet (cursor-paginated). |
| `Export(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/export` | GDPR data export one JSON download of everything on the account. |
| `CreateRefund(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/refunds` | Request a refund for a ticket. |
| `CreateReport(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/reports` | File a report against an event or organizer. |
| `Messages(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/messages` | The buyer's message threads with organizers. |
| `SendTicketMessage(ctx, ticketId, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/tickets/:ticketId/message` | Message the organizer about a ticket (rate-limited). |

### `client.Integrations` API keys

Manage your organization's own API keys (organizer owner/admin auth).

| Method | Endpoint | Description |
| --- | --- | --- |
| `CreateApiKey(ctx, orgId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys` | Create an API key (plaintext secret returned exactly once). |
| `ListApiKeys(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/integrations/org/:orgId/api-keys` | List API keys (prefixes and metadata only, never the secret). |
| `UpdateApiKey(ctx, orgId, keyId, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Update a key (rename, pause, re-scope). |
| `RotateApiKey(ctx, orgId, keyId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId/rotate` | Rotate a key's secret (new secret returned once; old one invalidated). |
| `DeleteApiKey(ctx, orgId, keyId, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Revoke an API key. |

### `client.Webhooks` Webhooks

Register webhook endpoints, manage secrets, inspect and replay deliveries. (Use webhooks.verify() to validate inbound signatures.)

| Method | Endpoint | Description |
| --- | --- | --- |
| `Create(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/webhooks` | Create a webhook endpoint (signing secret returned exactly once). |
| `List(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/webhooks` | List webhook endpoints. |
| `Update(ctx, id, opts...) (json.RawMessage, error)` | `PUT /api/v1/webhooks/:id` | Update a webhook endpoint. |
| `Delete(ctx, id, opts...) (json.RawMessage, error)` | `DELETE /api/v1/webhooks/:id` | Delete a webhook endpoint. |
| `Test(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/webhooks/:id/test` | Send a signed test event to the endpoint. |
| `RotateSecret(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/webhooks/:id/rotate-secret` | Rotate the signing secret (new secret returned once). |
| `Deliveries(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/webhooks/:id/deliveries` | List delivery attempts for an endpoint. |
| `Replay(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/webhooks/deliveries/:id/replay` | Replay a past delivery. |
| `Catalog(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/webhooks/catalog` | List every subscribable event type (no auth). |

<!-- END ENDPOINTS -->
