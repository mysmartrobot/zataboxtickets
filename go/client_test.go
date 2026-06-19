package zatabox

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := New("vt_live_x", WithBaseURL(srv.URL))
	return c, srv
}

func writeEnvelope(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]interface{}{"success": status < 400, "data": data, "meta": map[string]string{"request_id": "req_1"}}
	_ = json.NewEncoder(w).Encode(body)
}

func TestBaseURLRouting(t *testing.T) {
	if got := New("vt_test_x").baseURL; got != defaultSandbox {
		t.Fatalf("test key -> %s", got)
	}
	if got := New("vt_live_x").baseURL; got != defaultLive {
		t.Fatalf("live key -> %s", got)
	}
	if got := New("vt_test_x", WithBaseURL("http://localhost:4000")).baseURL; got != "http://localhost:4000" {
		t.Fatalf("override -> %s", got)
	}
}

func TestGetUnwrapsAndQuery(t *testing.T) {
	var gotPath, gotAuth, gotMethod string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		writeEnvelope(w, 200, map[string]interface{}{"items": []int{1}})
	})
	defer srv.Close()

	data, err := c.Events.List(context.Background(), WithQuery(map[string]interface{}{"limit": 20, "category": "music"}))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Items []int `json:"items"`
	}
	_ = json.Unmarshal(data, &out)
	if len(out.Items) != 1 || out.Items[0] != 1 {
		t.Fatalf("unwrap failed: %s", data)
	}
	if !strings.HasPrefix(gotPath, "/api/v1/events?") || !strings.Contains(gotPath, "limit=20") {
		t.Fatalf("path %s", gotPath)
	}
	if gotAuth != "Bearer vt_live_x" {
		t.Fatalf("auth %s", gotAuth)
	}
	if gotMethod != "GET" {
		t.Fatalf("method %s", gotMethod)
	}
}

func TestWriteBodyAndIdempotency(t *testing.T) {
	var gotIdem, gotCT string
	var gotBody map[string]interface{}
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		writeEnvelope(w, 201, map[string]interface{}{"id": "o1"})
	})
	defer srv.Close()

	if _, err := c.Orders.Create(context.Background(), WithBody(map[string]interface{}{"items": []interface{}{}})); err != nil {
		t.Fatal(err)
	}
	if gotIdem == "" {
		t.Fatal("expected auto idempotency key")
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type %s", gotCT)
	}
	if _, ok := gotBody["items"]; !ok {
		t.Fatalf("body %v", gotBody)
	}
}

func TestExplicitIdempotency(t *testing.T) {
	var gotIdem string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		writeEnvelope(w, 201, map[string]interface{}{})
	})
	defer srv.Close()
	_, _ = c.Orders.Create(context.Background(), WithBody(map[string]interface{}{}), WithIdempotencyKey("fixed"))
	if gotIdem != "fixed" {
		t.Fatalf("idem %s", gotIdem)
	}
}

func TestPathEncoding(t *testing.T) {
	var gotPath string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		writeEnvelope(w, 200, map[string]interface{}{})
	})
	defer srv.Close()
	_, _ = c.Events.Get(context.Background(), "a/b c")
	if !strings.Contains(gotPath, "/api/v1/events/a%2Fb%20c") {
		t.Fatalf("path %s", gotPath)
	}
}

func TestErrorMapping(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":"ORDER_NOT_FOUND","message":"nope","details":{"id":"x"}},"meta":{"request_id":"req_9"}}`))
	})
	defer srv.Close()
	c.maxRetries = 0
	_, err := c.Orders.Get(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	var e *Error
	if !asError(err, &e) {
		t.Fatalf("not *Error: %v", err)
	}
	if e.Code != "ORDER_NOT_FOUND" || e.Status != 404 || e.RequestID != "req_9" {
		t.Fatalf("error %+v", e)
	}
}

func TestRateLimited(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"success":false,"error":{}}`))
	})
	defer srv.Close()
	c.maxRetries = 0
	_, err := c.Events.List(context.Background())
	var e *Error
	if !asError(err, &e) || e.Code != "RATE_LIMITED" || e.RetryAfter != "30" {
		t.Fatalf("error %v", err)
	}
}

func TestBinary(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF"))
	})
	defer srv.Close()
	data, ct, err := c.Tickets.Pdf(context.Background(), "5")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "%PDF" || ct != "application/pdf" {
		t.Fatalf("got %q %q", data, ct)
	}
}

func TestPaginate(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "cursor=c2") {
			writeEnvelope(w, 200, map[string]interface{}{"items": []int{2}, "nextCursor": nil})
		} else {
			writeEnvelope(w, 200, map[string]interface{}{"items": []int{1}, "pagination": map[string]interface{}{"cursor": "c2"}})
		}
	})
	defer srv.Close()
	var ids []int
	err := c.Paginate(context.Background(), c.Events.List, map[string]interface{}{"limit": 10}, func(page json.RawMessage) error {
		var p struct {
			Items []int `json:"items"`
		}
		_ = json.Unmarshal(page, &p)
		ids = append(ids, p.Items...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(ids) != "[1 2]" {
		t.Fatalf("ids %v", ids)
	}
}

func TestSSEURL(t *testing.T) {
	c := New("vt_live_x")
	if got := c.Checkin.LiveURL("42"); got != "https://api.zatabox.com/api/v1/checkin/event/42/live" {
		t.Fatalf("sse url %s", got)
	}
}

func TestWebhookVerify(t *testing.T) {
	c := New("vt_live_x")
	secret := "whsec"
	ts := fmt.Sprint(time.Now().Unix())
	raw := []byte(`{"type":"order.paid"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(raw)))
	sig := hex.EncodeToString(mac.Sum(nil))
	ev, err := c.Webhooks.Verify(raw, "t="+ts+",v1="+sig, secret)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ev), "order.paid") {
		t.Fatalf("event %s", ev)
	}
	if _, err := c.Webhooks.Verify(raw, "t="+ts+",v1=00", secret); err == nil {
		t.Fatal("expected bad-signature error")
	}
	if _, err := c.Webhooks.Verify(raw, "", secret); err == nil {
		t.Fatal("expected missing-signature error")
	}
}

func TestSetBearerToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeEnvelope(w, 200, map[string]interface{}{})
	}))
	defer srv.Close()
	c := New("", WithBearerToken("jwt1"), WithBaseURL(srv.URL))
	c.SetBearerToken("jwt2")
	_, _ = c.Users.Me(context.Background())
	if gotAuth != "Bearer jwt2" {
		t.Fatalf("auth %s", gotAuth)
	}
}

func TestUpload(t *testing.T) {
	var gotName, gotContent, gotField, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Errorf("form file: %v", err)
		} else {
			b, _ := io.ReadAll(f)
			gotName = hdr.Filename
			gotContent = string(b)
		}
		gotField = r.FormValue("caption")
		writeEnvelope(w, 200, map[string]interface{}{"ok": true})
	}))
	defer srv.Close()
	c := New("vt_live_x", WithBaseURL(srv.URL))
	_, err := c.Media.Upload(context.Background(), []byte("PNGDATA"), UploadOptions{
		Filename: "cover.png", ContentType: "image/png", Fields: map[string]string{"caption": "hi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "cover.png" || gotContent != "PNGDATA" || gotField != "hi" {
		t.Fatalf("got name=%q content=%q field=%q", gotName, gotContent, gotField)
	}
	if !strings.HasPrefix(gotCT, "multipart/form-data; boundary=") {
		t.Fatalf("content-type %q", gotCT)
	}
}

// asError is a tiny errors.As shim kept local so the test is self-contained.
func asError(err error, target **Error) bool {
	if e, ok := err.(*Error); ok {
		*target = e
		return true
	}
	return false
}
