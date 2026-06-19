# zatabox-go Go SDK

Official **Go SDK** for the [Zatabox Tickets](https://zatabox.com) REST API the
white-label event-ticketing platform. A small, dependency-free client over
`https://api.zatabox.com/api/v1` that handles auth, sandbox routing, idempotency,
retries, pagination, binary downloads, file uploads and webhook verification.

- **Zero dependencies** standard library only (`net/http`, `crypto/hmac`).
- **Complete** every one of the **244 REST endpoints** is a method.
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
- [Binary downloads (PDF / CSV)](#binary-downloads-pdf--csv)
- [Live check-in stream (SSE)](#live-check-in-stream-sse)
- [File uploads](#file-uploads)
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

This SDK is **distributed via GitHub** — the Go module lives in the `go/` directory of
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
with your production account). A test key used against production or vice-versa —
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

`Auth`, `Users`, `SavedSearches`, `DataExport`, `Events`, `Tickets`, `Orders`,
`Payments`, `Checkin`, `Scan`, `Search`, `Media`, `Organizer`, `Wallets`,
`ScannerTokens`, `Integrations`, `EventCustomization`, `Growth`, `PublicEvents`,
`Site`, `Community`, `Track`, `Webhooks`, `WhiteLabel`, `Support`, `Util`.

Every method takes a `context.Context`, then any path params (strings), then variadic
`RequestOption`s. Bodies and query params are passed as options:

```go
z.Events.List(ctx, zatabox.WithQuery(map[string]interface{}{"q": "jazz", "limit": 20}))
z.Events.Get(ctx, "warehouse-sessions-004")
z.Organizer.UpdateTicket(ctx, eventID, ticketID, zatabox.WithBody(map[string]interface{}{"price": 7500}))
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
err := z.Paginate(ctx, z.Organizer.Events, map[string]interface{}{"limit": 50}, func(page json.RawMessage) error {
	var p struct{ Events []json.RawMessage `json:"events"` }
	_ = json.Unmarshal(page, &p)
	// ... handle p.Events
	return nil
})
```

Any method matching `func(ctx, ...RequestOption) (json.RawMessage, error)` (i.e. a list
endpoint) can be passed.

## Binary downloads (PDF / CSV)

`Tickets.Pdf`, `Orders.Invoice`, `Checkin.Export`, `Organizer.EventExport` and
`Site.Sitemap` return `([]byte, contentType string, error)`:

```go
pdf, contentType, err := z.Tickets.Pdf(ctx, ticketID)
_ = os.WriteFile("ticket.pdf", pdf, 0o644)
_ = contentType
```

## Live check-in stream (SSE)

`z.Checkin.LiveURL(eventID)` returns the stream URL; consume it with your SSE client
(passing `Authorization: Bearer <key>`).

## File uploads

```go
asset, err := z.Media.Upload(ctx, fileBytes, zatabox.UploadOptions{
	Filename:    "cover.jpg",
	ContentType: "image/jpeg",
	Fields:      map[string]string{"alt": "Cover"},
})
```

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

The SDK exposes **244 endpoints** across **26 namespaces**. Every method is listed below with its idiomatic signature, the underlying HTTP route, and what it does. Path parameters are positional; reads take an optional query map and writes take an optional body, both followed by a call-options bag.

### `client.Auth` Auth

Registration, password + passwordless sign-in, token refresh, 2FA and verification.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Register(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/register` | Register a new user with email + password. |
| `Login(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/login` | Sign in with email + password; returns the JWT pair (or a 2FA challenge). |
| `LoginVerify2fa(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/2fa-verify` | Complete a login that returned a 2FA challenge. |
| `Refresh(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/refresh` | Exchange a refresh token for a fresh access/refresh pair. |
| `Logout(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/logout` | Revoke a refresh token. |
| `ForgotPassword(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/forgot-password` | Email a password-reset link. |
| `ResetPassword(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/reset-password` | Set a new password using a reset token. |
| `RequestToken(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/token/request` | Passwordless: email a 6-digit login code. |
| `ExchangeToken(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/token/exchange` | Passwordless: swap an emailed code for the JWT pair. |
| `VerifyOtp(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/verify-otp` | Verify a one-time passcode. |
| `VerifyEmail(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/verify-email` | Confirm an email address from a verification token. |
| `VerifyPhone(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/verify-phone` | Confirm a phone number from an SMS code. |
| `LoginOauth(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/login/oauth` | Sign in with a third-party OAuth identity token. |
| `Enable2fa(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/2fa/enable` | Begin enrolling TOTP two-factor auth. |
| `Verify2fa(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/auth/2fa/verify` | Confirm a TOTP code to finish 2FA enrollment. |

### `client.Users` Users (Buyer)

The authenticated account: profile, wallet, tickets, orders, refunds, reports, messaging and notifications.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Me(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me` | Current user profile. |
| `UpdateMe(ctx, opts...) (json.RawMessage, error)` | `PUT /api/v1/users/me` | Update the current user profile. |
| `Orders(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/orders` | List the buyer's orders. |
| `Tickets(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/tickets` | List the buyer's tickets across all organizers. |
| `DeleteAccount(ctx, opts...) (json.RawMessage, error)` | `DELETE /api/v1/users/me` | Close the account. |
| `ChangePassword(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/password` | Change the account password. |
| `Activity(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/activity` | Recent account activity. |
| `LoginInfo(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/login-info` | Last-login metadata. |
| `TwofaStatus(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/2fa/status` | Whether 2FA is enabled. |
| `TwofaSetup(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/2fa/setup` | Start 2FA setup (returns the TOTP secret/QR). |
| `TwofaEnable(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/2fa/enable` | Enable 2FA after verifying a code. |
| `TwofaDisable(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/2fa/disable` | Disable 2FA. |
| `CreateRefund(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/refunds` | Submit a refund request for a ticket. |
| `Refunds(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/refunds` | List the buyer's refund requests. |
| `WithdrawRefund(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/refunds/:id/withdraw` | Withdraw a pending refund request. |
| `CreateReport(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/reports` | File a report against an event or organizer. |
| `Reports(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/reports` | List the buyer's filed reports. |
| `Messages(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/messages` | Message threads with organizers. |
| `MessageThread(ctx, threadId, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/messages/:threadId` | A single message thread. |
| `SendTicketMessage(ctx, ticketId, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/tickets/:ticketId/message` | Message the organizer of a ticket. |
| `Notifications(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/notifications` | In-app notifications. |
| `NotificationsUnreadCount(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/notifications/unread-count` | Count of unread notifications. |
| `MarkNotificationsRead(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/notifications/read` | Mark notifications as read. |
| `UpdateAvatar(ctx, opts...) (json.RawMessage, error)` | `PUT /api/v1/users/me/avatar` | Update the account avatar. |
| `UpdateNotificationSettings(ctx, opts...) (json.RawMessage, error)` | `PUT /api/v1/users/me/notifications/settings` | Update notification preferences. |

### `client.SavedSearches` Saved searches

The buyer's saved discovery searches.

| Method | Endpoint | Description |
| --- | --- | --- |
| `List(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/saved-searches` | List saved searches. |
| `Create(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/users/me/saved-searches` | Save a search. |
| `Delete(ctx, id, opts...) (json.RawMessage, error)` | `DELETE /api/v1/users/me/saved-searches/:id` | Delete a saved search. |

### `client.DataExport` Data export

GDPR-style export of the account's data.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Get(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/users/me/export` | Export all of the account's data. |

### `client.Events` Events (Public)

Public event discovery rails and per-event read panels.

| Method | Endpoint | Description |
| --- | --- | --- |
| `List(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/events` | List / search public events (cursor-paginated). |
| `Trending(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/events/trending` | Trending events. |
| `Categories(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/events/categories` | Event categories with counts. |
| `Nearby(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/events/nearby` | Events near a lat/lng or city. |
| `NewThisWeek(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/events/new-this-week` | Recently published events. |
| `EndingSoon(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/events/ending-soon` | Events with sales ending soon. |
| `Free(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/events/free` | Free events. |
| `Recommended(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/events/recommended` | Personalized recommendations. |
| `Schedule(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/events/:id/schedule` | An event's session schedule. |
| `Organizer(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/events/:id/organizer` | Public organizer profile for an event. |
| `Faq(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/events/:id/faq` | An event's FAQ. |
| `Related(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/events/:id/related` | Related events. |
| `ExpressInterest(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/events/:id/interest` | Register interest in an event. |
| `Issue(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/events/:eventId/issue` | Externally issue a ticket for an event (integrator mode). |
| `Get(ctx, slug, opts...) (json.RawMessage, error)` | `GET /api/v1/events/:slug` | Get a public event by slug. |
| `Tickets(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/events/:id/tickets` | List an event's purchasable ticket types. |

### `client.Tickets` Tickets

Ticket QR/PDF, peer-to-peer transfers, promo validation and wallet passes.

| Method | Endpoint | Description |
| --- | --- | --- |
| `ValidatePromo(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/tickets/promo/validate` | Validate a promo code against a cart. |
| `Qr(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/tickets/:id/qr` | Current rotating QR payload for a ticket (JSON). |
| `Pdf(ctx, id, opts...) ([]byte, string, error)` | `GET /api/v1/tickets/:id/pdf` | Ticket PDF (application/pdf bytes). |
| `Transfer(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/tickets/:id/transfer` | Initiate a peer-to-peer transfer; the recipient gets a claim link. |
| `RevokeTransfer(ctx, transferId, opts...) (json.RawMessage, error)` | `POST /api/v1/tickets/transfers/:transferId/revoke` | Revoke a still-pending transfer (initiator only). |
| `GetTransfer(ctx, token, opts...) (json.RawMessage, error)` | `GET /api/v1/tickets/transfers/claim/:token` | Inspect a pending transfer by claim token. |
| `ClaimTransfer(ctx, token, opts...) (json.RawMessage, error)` | `POST /api/v1/tickets/transfers/claim/:token` | Claim a transfer; rewrites the ticket holder to the recipient. |
| `WalletPass(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/tickets/:id/wallet-pass` | Google/Apple wallet 'Save to Wallet' link (JSON). |
| `ListByEvent(ctx, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/tickets/:eventId` | List ticket types for an event (legacy /ticket-types mount). |

### `client.Orders` Orders

Carted checkout: create, pay, cancel, invoice.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Create(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/orders` | Create an order (reserves inventory). |
| `Get(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/orders/:id` | Order detail. |
| `Cancel(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/orders/:id/cancel` | Cancel an order and release its hold. |
| `Pay(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/orders/:id/pay` | Initiate payment for an order (nowpayments \| paystack \| flutterwave). |
| `Invoice(ctx, id, opts...) ([]byte, string, error)` | `GET /api/v1/orders/:id/invoice` | Order receipt PDF (application/pdf bytes). |

### `client.Payments` Payments

Payment intent creation, verification, method discovery, and inbound provider webhooks.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Verify(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/payments/verify` | Confirm a charge with the provider and complete the order (idempotent). |
| `Get(ctx, orderId, opts...) (json.RawMessage, error)` | `GET /api/v1/payments/:orderId` | Payment status for an order. |
| `Initiate(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/payments/initiate` | Create a payment intent. |
| `Methods(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/payments/methods` | Available payment methods for a currency. |
| `CryptoCurrencies(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/payments/crypto/currencies` | Supported crypto currencies (NOWPayments). |
| `WebhookNowpayments(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/payments/webhook/nowpayments` | Inbound NOWPayments webhook (provider callback; not for client use). |
| `WebhookPaystack(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/payments/webhook/paystack` | Inbound Paystack webhook (provider callback; not for client use). |
| `WebhookFlutterwave(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/payments/webhook/flutterwave` | Inbound Flutterwave webhook (provider callback; not for client use). |

### `client.Checkin` Check-in

Gate scanning, offline manifests, batch sync, live stats and CSV export.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Scan(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/checkin/scan` | Validate a QR / short-code ticket at a gate. |
| `Batch(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/checkin/batch` | Flush a queue of scans captured offline. |
| `Manual(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/checkin/event/:id/manual` | Manually check in an attendee. |
| `Manifest(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/checkin/event/:id/manifest` | Offline manifest (ticket hashes + statuses); pass ?since for a delta. |
| `Stats(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/checkin/event/:id/stats` | Live check-in stats snapshot. |
| `RegisterDevice(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/checkin/device/register` | Register a scanning device. |
| `LiveURL(id, query...) string` | `GET /api/v1/checkin/event/:id/live` | Server-Sent Events stream of live check-in stats. |
| `Gate(ctx, id, gate, opts...) (json.RawMessage, error)` | `GET /api/v1/checkin/event/:id/gate/:gate` | Per-gate check-in stats. |
| `Export(ctx, id, opts...) ([]byte, string, error)` | `GET /api/v1/checkin/event/:id/export` | Check-in log CSV (text/csv bytes). |

### `client.Scan` Scanner-token check-in

Passwordless gate scanning using a short-lived scanner token (the /scan kiosk surface).

| Method | Endpoint | Description |
| --- | --- | --- |
| `Exchange(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/checkin-token/exchange` | Exchange a scanner token for a scoped session. |
| `Session(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/checkin-token/me` | Current scanner session (event + gate context). |
| `Scan(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/checkin-token/scan` | Validate a ticket with the scanner session. |
| `Manifest(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/checkin-token/manifest` | Offline manifest for the scanner session. |
| `Batch(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/checkin-token/batch` | Flush offline scans for the scanner session. |

### `client.Search` Search

Full-text + faceted event search.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Query(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/search` | Full-text + faceted search. |
| `Suggest(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/search/suggest` | Type-ahead suggestions. |
| `Trending(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/search/trending` | Trending search terms. |
| `Popular(ctx, city, opts...) (json.RawMessage, error)` | `GET /api/v1/search/popular/:city` | Popular searches in a city. |

### `client.Media` Media

Image/asset uploads and the stable /media/:id resolver.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Upload(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/media/upload` | Upload an image/asset (multipart/form-data). |

### `client.Organizer` Organizer

Authenticated organizer surface: organizations, members, events, ticket types, schedules, sections, promo codes, attendees, payouts, refunds, reports and messaging.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Setup(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/setup` | Bootstrap an organizer account + first organization. |
| `Me(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/me` | Current organizer context (orgs + memberships). |
| `CreateOrganization(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/organizations` | Create an organization. |
| `GetOrganization(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/organizations/:orgId` | Organization detail. |
| `UpdateOrganization(ctx, orgId, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/organizations/:orgId` | Update an organization. |
| `SetOrganizationStatus(ctx, orgId, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/organizations/:orgId/status` | Change an organization's status. |
| `Members(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/organizations/:orgId/members` | List organization members. |
| `Invite(ctx, orgId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/organizations/:orgId/invites` | Invite a member. |
| `ResendInvite(ctx, orgId, memberId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/organizations/:orgId/invites/:memberId/resend` | Resend a member invite. |
| `RemoveMember(ctx, orgId, memberId, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/organizations/:orgId/members/:memberId` | Remove a member. |
| `Events(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/events` | List the organizer's events. |
| `CreateEvent(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events` | Create a draft event. |
| `GetEvent(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/events/:id` | Organizer event detail. |
| `UpdateEvent(ctx, id, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/events/:id` | Update an event. |
| `SetEventStatus(ctx, id, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/events/:id/status` | Change an event's status. |
| `PublishEvent(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events/:id/publish` | Publish a draft event. |
| `UnpublishEvent(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events/:id/unpublish` | Unpublish a published event back to draft. |
| `DeleteEvent(ctx, id, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/events/:id` | Cancel/delete an event. |
| `Tickets(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/events/:id/tickets` | List an event's ticket types. |
| `CreateTicket(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events/:id/tickets` | Create a ticket type. |
| `UpdateTicket(ctx, id, tid, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/events/:id/tickets/:tid` | Update a ticket type. |
| `DeleteTicket(ctx, id, tid, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/events/:id/tickets/:tid` | Delete a ticket type. |
| `Schedule(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/events/:id/schedule` | List schedule sessions. |
| `CreateSchedule(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events/:id/schedule` | Add a schedule session. |
| `UpdateSchedule(ctx, id, sid, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/events/:id/schedule/:sid` | Update a schedule session. |
| `DeleteSchedule(ctx, id, sid, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/events/:id/schedule/:sid` | Delete a schedule session. |
| `Sections(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/events/:id/sections` | List seating/venue sections. |
| `CreateSection(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/events/:id/sections` | Add a section. |
| `UpdateSection(ctx, id, sid, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/events/:id/sections/:sid` | Update a section. |
| `DeleteSection(ctx, id, sid, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/events/:id/sections/:sid` | Delete a section. |
| `PromoCodes(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/promo-codes` | List promo codes. |
| `CreatePromoCode(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/promo-codes` | Create a promo code. |
| `UpdatePromoCode(ctx, id, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/promo-codes/:id` | Update a promo code. |
| `DeletePromoCode(ctx, id, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/promo-codes/:id` | Delete a promo code. |
| `EventAnalytics(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/events/:id/analytics` | Sales/analytics for an event. |
| `EventAttendees(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/events/:id/attendees` | Attendee CRM list for an event. |
| `EventExport(ctx, id, opts...) ([]byte, string, error)` | `GET /api/v1/organizer/events/:id/export` | Attendee export CSV (text/csv bytes). |
| `Payouts(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/payouts` | List payouts. |
| `Payout(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/payouts/:id` | Payout detail. |
| `RequestPayout(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/payouts/request` | Request a payout. |
| `UpdatePayoutSettings(ctx, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/payout-settings` | Update payout settings. |
| `Refunds(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/orgs/:orgId/refunds` | List refund requests for an org. |
| `DecideRefund(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/refunds/:id/decide` | Approve or deny a refund request. |
| `Reports(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/orgs/:orgId/reports` | List reports filed against an org (read-only). |
| `ResolveReport(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/reports/:id/resolve` | Resolve a report (admins only; organizers receive 403). |
| `Messages(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/orgs/:orgId/messages` | Org-wide buyer message threads. |
| `Overview(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/orgs/:orgId/overview` | Org dashboard overview. |
| `Referral(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/orgs/:orgId/referral` | Referral program summary + commissions. |
| `Notifications(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/orgs/:orgId/notifications` | Org notifications. |
| `FundWallet(ctx, orgId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/wallets/org/:orgId/fund` | Top up the org wallet (returns a payment intent). |
| `FundWalletStatus(ctx, orgId, orderId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/wallets/org/:orgId/fund/:orderId` | Poll a wallet top-up payment. |
| `PayoutDetails(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/orgs/:orgId/payout-details` | Stored payout destinations. |
| `UpdatePayoutDetails(ctx, orgId, currency, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/orgs/:orgId/payout-details/:currency` | Set payout details for a currency. |
| `TicketMessage(ctx, ticketId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/tickets/:ticketId/message` | Reply to a buyer on a ticket thread. |
| `MessageThread(ctx, threadId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/messages/:threadId` | A single org message thread. |

### `client.Wallets` Wallets

Organization wallet balances and ledger.

| Method | Endpoint | Description |
| --- | --- | --- |
| `List(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/wallets` | List wallets the caller can see. |
| `Get(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/wallets/org/:orgId` | An org's wallet balances (per currency). |
| `Bootstrap(ctx, orgId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/wallets/org/:orgId/bootstrap` | Provision an org's wallet. |
| `Transactions(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/wallets/org/:orgId/transactions` | Wallet ledger transactions. |

### `client.ScannerTokens` Scanner tokens

Manage short-lived gate-scanner tokens for staff devices.

| Method | Endpoint | Description |
| --- | --- | --- |
| `List(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/scanner-tokens/org/:orgId` | List scanner tokens. |
| `Create(ctx, orgId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/scanner-tokens/org/:orgId` | Mint a scanner token. |
| `Update(ctx, orgId, tokenId, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId` | Update a scanner token. |
| `Delete(ctx, orgId, tokenId, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId` | Revoke a scanner token. |
| `Reissue(ctx, orgId, tokenId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId/reissue` | Reissue a scanner token's secret. |
| `Metrics(ctx, orgId, tokenId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/scanner-tokens/org/:orgId/:tokenId/metrics` | Usage metrics for a scanner token. |

### `client.Integrations` Integrations

API keys, MCP tokens, and integration usage metrics.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Metrics(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/integrations/org/:orgId/metrics` | Integration usage metrics. |
| `ApiCalls(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/integrations/org/:orgId/api-calls` | Recent API call log. |
| `McpCalls(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/integrations/org/:orgId/mcp-calls` | Recent MCP tool-call log. |
| `ListApiKeys(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/integrations/org/:orgId/api-keys` | List API keys. |
| `CreateApiKey(ctx, orgId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys` | Mint an API key (plaintext returned once). |
| `UpdateApiKey(ctx, orgId, keyId, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Update an API key (scopes, status, allowlist). |
| `RotateApiKey(ctx, orgId, keyId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId/rotate` | Rotate an API key's secret. |
| `DeleteApiKey(ctx, orgId, keyId, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/integrations/org/:orgId/api-keys/:keyId` | Revoke an API key. |
| `ListMcpTokens(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/integrations/org/:orgId/mcp-tokens` | List MCP tokens. |
| `CreateMcpToken(ctx, orgId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/integrations/org/:orgId/mcp-tokens` | Mint an MCP token. |
| `UpdateMcpToken(ctx, orgId, tokenId, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/integrations/org/:orgId/mcp-tokens/:tokenId` | Update an MCP token. |
| `DeleteMcpToken(ctx, orgId, tokenId, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/integrations/org/:orgId/mcp-tokens/:tokenId` | Revoke an MCP token. |

### `client.EventCustomization` Event customization

Per-event white-label theming.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Get(ctx, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/event-customization/:eventId` | Get an event's customization. |
| `Update(ctx, eventId, opts...) (json.RawMessage, error)` | `PUT /api/v1/organizer/event-customization/:eventId` | Update an event's customization. |

### `client.Growth` Growth

Comp tickets, CSV import, broadcasts and attendee tags.

| Method | Endpoint | Description |
| --- | --- | --- |
| `MintComps(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/growth/events/:eventId/comps` | Issue complimentary tickets. |
| `ImportCompsCsv(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/growth/events/:eventId/comps/import-csv` | Bulk-issue comps from CSV. |
| `ListComps(ctx, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/growth/events/:eventId/comps` | List issued comps. |
| `ResendComp(ctx, eventId, ticketId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/growth/events/:eventId/comps/:ticketId/resend` | Resend a comp ticket email. |
| `BroadcastEvent(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/growth/events/:eventId/broadcast` | Broadcast to an event's attendees. |
| `BroadcastOrg(ctx, orgId, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/growth/orgs/:orgId/broadcast` | Broadcast to an org's followers/subscribers. |
| `ListBroadcasts(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/growth/orgs/:orgId/broadcasts` | List sent broadcasts. |
| `AddTags(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/organizer/growth/tags` | Tag attendees (body: orgId, ticketIds, tag). |
| `RemoveTag(ctx, opts...) (json.RawMessage, error)` | `DELETE /api/v1/organizer/growth/tags` | Remove a tag (body: orgId, ticketId, tag). |
| `ListTags(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/growth/orgs/:orgId/tags` | List tags for an org. |
| `TagAttendees(ctx, orgId, tag, opts...) (json.RawMessage, error)` | `GET /api/v1/organizer/growth/orgs/:orgId/tags/:tag/attendees` | List attendees with a given tag. |

### `client.PublicEvents` Public events (vanity)

Public org/event read endpoints used by hosted pages and white-label sites.

| Method | Endpoint | Description |
| --- | --- | --- |
| `GetBySlug(ctx, slug, opts...) (json.RawMessage, error)` | `GET /api/v1/public/events/:slug` | Public event by slug. |
| `OrgEvents(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/public/events/orgs/by/:orgId` | An organization's public events. |
| `GetByOrgEvent(ctx, orgId, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/public/events/by/:orgId/:eventId` | Public event by org + event id. |
| `GetById(ctx, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/public/events/by-id/:eventId` | Public event by id. |
| `Preview(ctx, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/public/events/preview/:eventId` | Draft event preview (token-gated). |

### `client.Site` Public site

Sitemap, newsletter signup and platform status.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Sitemap(ctx, opts...) ([]byte, string, error)` | `GET /api/v1/public/sitemap.xml` | Sitemap XML (application/xml bytes). |
| `NewsletterStart(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/public/newsletter/start` | Begin newsletter double-opt-in. |
| `NewsletterConfirm(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/public/newsletter` | Confirm a newsletter subscription. |
| `Status(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/public/status` | Platform status + incidents. |

### `client.Community` Community

Verified-attendee reviews, organizer follows and event waitlists.

| Method | Endpoint | Description |
| --- | --- | --- |
| `SubmitReview(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/community/reviews` | Submit a verified-attendee review (ticketCode + email prove ownership). |
| `OrgReviews(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/community/orgs/:orgId/reviews` | Published reviews for an org. |
| `EventReviews(ctx, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/community/events/:eventId/reviews` | Published reviews for an event. |
| `Follow(ctx, orgId, opts...) (json.RawMessage, error)` | `POST /api/v1/community/orgs/:orgId/follow` | Follow an organizer. |
| `Unfollow(ctx, token, opts...) (json.RawMessage, error)` | `GET /api/v1/community/unfollow/:token` | Unsubscribe via emailed token. |
| `JoinWaitlist(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/community/events/:eventId/waitlist` | Join an event waitlist. |
| `AcceptWaitlist(ctx, id, token, opts...) (json.RawMessage, error)` | `GET /api/v1/community/waitlist/accept/:id/:token` | Accept a waitlist offer via emailed token. |
| `ManageReviews(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/community/orgs/:orgId/reviews/manage` | Organizer view of all reviews (incl. pending). |
| `ReplyReview(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/community/reviews/:id/reply` | Organizer reply to a review. |
| `SetReviewStatus(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/community/reviews/:id/status` | Publish/hide a review. |
| `Followers(ctx, orgId, opts...) (json.RawMessage, error)` | `GET /api/v1/community/orgs/:orgId/followers` | List an org's followers. |
| `RemoveFollower(ctx, orgId, followerId, opts...) (json.RawMessage, error)` | `DELETE /api/v1/community/orgs/:orgId/followers/:followerId` | Remove a follower. |
| `EventWaitlist(ctx, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/community/events/:eventId/waitlist` | List an event's waitlist. |
| `OfferWaitlist(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/community/events/:eventId/waitlist/offer` | Offer spots to waitlisted attendees. |

### `client.Track` Tracking

Lightweight page-view tracking.

| Method | Endpoint | Description |
| --- | --- | --- |
| `View(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/track/view` | Record a page view. |

### `client.Webhooks` Webhooks

Register webhook endpoints and inspect/replay deliveries. (Use client.webhooks.verify() to validate inbound signatures.)

| Method | Endpoint | Description |
| --- | --- | --- |
| `Catalog(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/webhooks/catalog` | List subscribable event types. |
| `List(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/webhooks` | List registered webhook endpoints. |
| `Create(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/webhooks` | Register a webhook endpoint. |
| `Update(ctx, id, opts...) (json.RawMessage, error)` | `PUT /api/v1/webhooks/:id` | Update a webhook endpoint. |
| `RotateSecret(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/webhooks/:id/rotate-secret` | Rotate a webhook signing secret. |
| `Delete(ctx, id, opts...) (json.RawMessage, error)` | `DELETE /api/v1/webhooks/:id` | Delete a webhook endpoint. |
| `Deliveries(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/webhooks/:id/deliveries` | List delivery attempts for an endpoint. |
| `Test(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/webhooks/:id/test` | Send a test event to an endpoint. |
| `Replay(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/webhooks/deliveries/:id/replay` | Replay a past delivery. |

### `client.WhiteLabel` White-label

Self-contained white-label surface: events, ticket types, customization, orders, tickets and check-in under one integration.

| Method | Endpoint | Description |
| --- | --- | --- |
| `Me(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/white-label/me` | The integration's white-label context. |
| `Events(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/white-label/events` | List white-label events. |
| `CreateEvent(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/white-label/events` | Create a white-label event. |
| `GetEvent(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/white-label/events/:id` | White-label event detail. |
| `UpdateEvent(ctx, id, opts...) (json.RawMessage, error)` | `PUT /api/v1/white-label/events/:id` | Update a white-label event. |
| `DeleteEvent(ctx, id, opts...) (json.RawMessage, error)` | `DELETE /api/v1/white-label/events/:id` | Delete a white-label event. |
| `TicketTypes(ctx, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/white-label/events/:eventId/ticket-types` | List ticket types. |
| `CreateTicketType(ctx, eventId, opts...) (json.RawMessage, error)` | `POST /api/v1/white-label/events/:eventId/ticket-types` | Create a ticket type. |
| `GetCustomization(ctx, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/white-label/events/:eventId/customization` | Get event customization. |
| `UpdateCustomization(ctx, eventId, opts...) (json.RawMessage, error)` | `PUT /api/v1/white-label/events/:eventId/customization` | Update event customization. |
| `Orders(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/white-label/orders` | List white-label orders. |
| `Tickets(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/white-label/tickets` | List white-label tickets. |
| `CheckinScan(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/white-label/checkin/scan` | Validate a ticket at a white-label gate. |
| `CheckinStats(ctx, eventId, opts...) (json.RawMessage, error)` | `GET /api/v1/white-label/checkin/stats/:eventId` | White-label check-in stats. |
| `Wallets(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/white-label/wallets` | White-label wallet balances. |

### `client.Support` Support

Support tickets and threaded messages.

| Method | Endpoint | Description |
| --- | --- | --- |
| `List(ctx, opts...) (json.RawMessage, error)` | `GET /api/v1/support` | List support tickets. |
| `Create(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/support` | Open a support ticket. |
| `Get(ctx, id, opts...) (json.RawMessage, error)` | `GET /api/v1/support/:id` | Support ticket detail. |
| `SendMessage(ctx, id, opts...) (json.RawMessage, error)` | `POST /api/v1/support/:id/messages` | Reply on a support ticket. |

### `client.Util` Utilities

Helper endpoints.

| Method | Endpoint | Description |
| --- | --- | --- |
| `ResolveCoords(ctx, opts...) (json.RawMessage, error)` | `POST /api/v1/util/resolve-coords` | Resolve an address to coordinates. |

<!-- END ENDPOINTS -->
