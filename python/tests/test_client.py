"""Smoke tests for the zatabox Python SDK. Run: python -m unittest discover tests"""

import io
import json
import time
import hmac
import hashlib
import unittest
from urllib.error import HTTPError

import zatabox
from zatabox import Client, ZataboxError
import zatabox._http as _http


class FakeResp:
    def __init__(self, body=b"", headers=None):
        self._body = body
        self.headers = _Headers(headers or {})

    def read(self):
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False


class _Headers(dict):
    def get(self, key, default=None):
        for k, v in self.items():
            if k.lower() == key.lower():
                return v
        return default


def patch_urlopen(fn):
    """Decorator: replace _http.urlopen with fn for the duration of a test."""
    def wrap(test):
        def inner(self):
            orig = _http.urlopen
            _http.urlopen = fn
            try:
                return test(self)
            finally:
                _http.urlopen = orig
        return inner
    return wrap


def json_ok(obj, status=200, headers=None):
    return FakeResp(json.dumps(obj).encode(), headers)


def http_error(status, obj, headers=None):
    body = json.dumps(obj).encode()
    return HTTPError("http://x", status, "err", _Headers(headers or {}), io.BytesIO(body))


class TestClient(unittest.TestCase):
    def test_requires_credential(self):
        with self.assertRaises(ValueError):
            Client()

    def test_base_url_routing(self):
        self.assertEqual(Client(api_key="vt_test_x").base_url, "https://sandbox.zatabox.com")
        self.assertEqual(Client(api_key="vt_live_x").base_url, "https://api.zatabox.com")
        self.assertEqual(Client(api_key="sk_test_x").base_url, "https://sandbox.zatabox.com")
        self.assertEqual(Client(api_key="vt_test_x", base_url="http://localhost:4000").base_url, "http://localhost:4000")

    def test_namespaces_present(self):
        c = Client(api_key="vt_live_x")
        for ns in ["auth", "users", "events", "orders", "checkin", "organizer",
                   "webhooks", "community", "growth", "white_label",
                   "saved_searches", "public_events", "media"]:
            self.assertTrue(hasattr(c, ns), "missing namespace %s" % ns)
        self.assertTrue(callable(c.events.list))
        self.assertTrue(callable(c.webhooks.verify))
        self.assertTrue(callable(c.media.upload))
        self.assertTrue(callable(c.checkin.live_url))

    def test_admin_not_exposed(self):
        # The /api/v1/admin/* surface must never ship in this public SDK.
        c = Client(api_key="vt_live_x")
        self.assertFalse(hasattr(c, "admin"), "admin namespace must not be exposed")

    def test_get_unwraps_and_builds_query(self):
        captured = {}

        def fake(req, timeout=None):
            captured["url"] = req.full_url
            captured["method"] = req.get_method()
            captured["auth"] = req.get_header("Authorization")
            return json_ok({"success": True, "data": {"items": [1]}})

        @patch_urlopen(fake)
        def run(self):
            c = Client(api_key="vt_live_x")
            data = c.events.list({"limit": 20, "category": "music"})
            self.assertEqual(data, {"items": [1]})
            self.assertIn("/api/v1/events?", captured["url"])
            self.assertIn("limit=20", captured["url"])
            self.assertEqual(captured["method"], "GET")
            self.assertEqual(captured["auth"], "Bearer vt_live_x")

        run(self)

    def test_write_sends_body_and_idempotency(self):
        captured = {}

        def fake(req, timeout=None):
            captured["body"] = req.data
            captured["idem"] = req.get_header("Idempotency-key")
            captured["ct"] = req.get_header("Content-type")
            return json_ok({"success": True, "data": {"id": "o1"}}, 201)

        @patch_urlopen(fake)
        def run(self):
            c = Client(api_key="vt_live_x")
            out = c.orders.create({"items": []})
            self.assertEqual(out, {"id": "o1"})
            self.assertEqual(json.loads(captured["body"]), {"items": []})
            self.assertTrue(captured["idem"])
            self.assertEqual(captured["ct"], "application/json")

        run(self)

    def test_explicit_idempotency_key(self):
        captured = {}

        def fake(req, timeout=None):
            captured["idem"] = req.get_header("Idempotency-key")
            return json_ok({"success": True, "data": {}}, 201)

        @patch_urlopen(fake)
        def run(self):
            Client(api_key="vt_live_x").orders.create({"items": []}, idempotency_key="fixed")
            self.assertEqual(captured["idem"], "fixed")

        run(self)

    def test_path_params_encoded(self):
        captured = {}

        def fake(req, timeout=None):
            captured["url"] = req.full_url
            return json_ok({"success": True, "data": {}})

        @patch_urlopen(fake)
        def run(self):
            Client(api_key="vt_live_x").events.get("a/b c")
            self.assertIn("/api/v1/events/a%2Fb%20c", captured["url"])

        run(self)

    def test_error_raises_zataboxerror(self):
        def fake(req, timeout=None):
            raise http_error(404, {"success": False, "error": {"code": "ORDER_NOT_FOUND", "message": "nope", "details": {"id": "x"}}, "meta": {"request_id": "req_1"}})

        @patch_urlopen(fake)
        def run(self):
            c = Client(api_key="vt_live_x", max_retries=0)
            try:
                c.orders.get("x")
                self.fail("expected error")
            except ZataboxError as e:
                self.assertEqual(e.code, "ORDER_NOT_FOUND")
                self.assertEqual(e.status, 404)
                self.assertEqual(e.request_id, "req_1")
                self.assertEqual(e.details, {"id": "x"})

        run(self)

    def test_429_rate_limited(self):
        def fake(req, timeout=None):
            raise http_error(429, {"success": False, "error": {}}, {"Retry-After": "30"})

        @patch_urlopen(fake)
        def run(self):
            try:
                Client(api_key="vt_live_x", max_retries=0).events.list()
                self.fail("expected")
            except ZataboxError as e:
                self.assertEqual(e.code, "RATE_LIMITED")
                self.assertEqual(e.details.get("retryAfter"), 30)

        run(self)

    def test_binary(self):
        def fake(req, timeout=None):
            return FakeResp(b"%PDF", {"Content-Type": "application/pdf", "Content-Disposition": 'inline; filename="t.pdf"'})

        @patch_urlopen(fake)
        def run(self):
            r = Client(api_key="vt_live_x").tickets.pdf("5")
            self.assertEqual(r["data"], b"%PDF")
            self.assertEqual(r["content_type"], "application/pdf")
            self.assertEqual(r["filename"], "t.pdf")

        run(self)

    def test_paginate(self):
        def fake(req, timeout=None):
            if "cursor=c2" in req.full_url:
                return json_ok({"success": True, "data": {"items": [2], "nextCursor": None}})
            return json_ok({"success": True, "data": {"items": [1], "pagination": {"cursor": "c2"}}})

        @patch_urlopen(fake)
        def run(self):
            c = Client(api_key="vt_live_x")
            ids = []
            for page in c.paginate(c.events.list, {"limit": 10}):
                ids.extend(page["items"])
            self.assertEqual(ids, [1, 2])

        run(self)

    def test_sse_url(self):
        c = Client(api_key="vt_live_x")
        self.assertEqual(c.checkin.live_url("42"), "https://api.zatabox.com/api/v1/checkin/event/42/live")

    def test_webhook_verify(self):
        c = Client(api_key="vt_live_x")
        secret = "whsec"
        t = int(time.time())
        raw = json.dumps({"type": "order.paid"})
        sig = hmac.new(secret.encode(), ("%s.%s" % (t, raw)).encode(), hashlib.sha256).hexdigest()
        ev = c.webhooks.verify(raw, "t=%s,v1=%s" % (t, sig), secret)
        self.assertEqual(ev["type"], "order.paid")
        with self.assertRaises(ZataboxError):
            c.webhooks.verify(raw, "t=%s,v1=00" % t, secret)
        with self.assertRaises(ZataboxError):
            c.webhooks.verify(raw, "", secret)

    def test_set_bearer_token(self):
        captured = {}

        def fake(req, timeout=None):
            captured["auth"] = req.get_header("Authorization")
            return json_ok({"success": True, "data": {}})

        @patch_urlopen(fake)
        def run(self):
            c = Client(bearer_token="jwt1")
            c.set_bearer_token("jwt2")
            c.users.me()
            self.assertEqual(captured["auth"], "Bearer jwt2")

        run(self)

    def test_upload_multipart(self):
        captured = {}

        def fake(req, timeout=None):
            captured["body"] = req.data
            captured["ct"] = req.get_header("Content-type")
            return json_ok({"success": True, "data": {"ok": True}})

        @patch_urlopen(fake)
        def run(self):
            Client(api_key="vt_live_x").media.upload(
                b"PNGDATA", filename="cover.png", content_type="image/png", fields={"caption": "hi"}
            )
            body = captured["body"]
            self.assertTrue(captured["ct"].startswith("multipart/form-data; boundary="))
            self.assertIn(b'name="file"; filename="cover.png"', body)
            self.assertIn(b"Content-Type: image/png", body)
            self.assertIn(b"PNGDATA", body)
            self.assertIn(b'name="caption"', body)
            self.assertIn(b"hi", body)

        run(self)

    def test_version(self):
        self.assertRegex(zatabox.__version__, r"^\d+\.\d+\.\d+")


if __name__ == "__main__":
    unittest.main()
