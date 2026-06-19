"""Low-level transport for the Zatabox SDK Python standard library only.

urllib + hmac + hashlib; no third-party dependencies. See ../../spec/CONVENTIONS.md.
"""

import json
import time
import uuid
import hmac
import hashlib
from urllib.parse import quote, urlencode, urlsplit, urlunsplit
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError

__version__ = "0.3.0"

DEFAULT_LIVE = "https://api.zatabox.com"
DEFAULT_SANDBOX = "https://sandbox.zatabox.com"


def enc(value):
    """URL-encode a single path segment."""
    return quote(str(value), safe="")


class ZataboxError(Exception):
    """Raised for any non-2xx response (and transport failures).

    Attributes: code, message, status, request_id, details.
    """

    def __init__(self, code=None, message=None, status=0, request_id=None, details=None):
        super().__init__(message or code or "Zatabox request failed")
        self.code = code or "UNKNOWN_ERROR"
        self.message = message or self.code
        self.status = status
        self.request_id = request_id
        self.details = details

    def __repr__(self):
        return "ZataboxError(code=%r, status=%r, request_id=%r)" % (
            self.code, self.status, self.request_id,
        )


def _resolve_base_url(api_key, base_url):
    if base_url:
        return base_url.rstrip("/")
    if api_key and (api_key.startswith("vt_test_") or api_key.startswith("sk_test_")):
        return DEFAULT_SANDBOX
    return DEFAULT_LIVE


def _build_query(query):
    pairs = []
    for key, val in (query or {}).items():
        if val is None:
            continue
        if isinstance(val, (list, tuple)):
            for item in val:
                pairs.append((key, str(item)))
        elif isinstance(val, bool):
            pairs.append((key, "true" if val else "false"))
        else:
            pairs.append((key, str(val)))
    return urlencode(pairs)


class Transport:
    """Shared HTTP plumbing; mixed into the Client."""

    def __init__(self, api_key=None, bearer_token=None, base_url=None,
                 timeout=30.0, max_retries=2, user_agent=None):
        if not api_key and not bearer_token:
            raise ValueError("zatabox: pass either api_key or bearer_token")
        self.api_key = api_key
        self._bearer_token = bearer_token
        self.base_url = _resolve_base_url(api_key, base_url)
        self.timeout = timeout
        self.max_retries = max_retries
        self.user_agent = user_agent or ("zatabox-python/%s" % __version__)

    @property
    def token(self):
        return self.api_key or self._bearer_token

    def set_bearer_token(self, token):
        self._bearer_token = token
        return self

    def _url(self, path, query=None):
        url = self.base_url + path
        qs = _build_query(query)
        if qs:
            scheme, netloc, p, existing, frag = urlsplit(url)
            merged = existing + ("&" if existing else "") + qs
            url = urlunsplit((scheme, netloc, p, merged, frag))
        return url

    def _request(self, method, path, query=None, body=None,
                 idempotency_key=None, headers=None, raw=False):
        url = self._url(path, query)
        hdrs = {
            "Accept": "*/*" if raw else "application/json",
            "User-Agent": self.user_agent,
            "Authorization": "Bearer %s" % self.token,
        }
        if headers:
            hdrs.update(headers)
        data = None
        if body is not None and method != "GET":
            data = json.dumps(body).encode("utf-8")
            hdrs["Content-Type"] = "application/json"
        if method != "GET" and "Idempotency-Key" not in hdrs:
            hdrs["Idempotency-Key"] = idempotency_key or str(uuid.uuid4())

        last_err = None
        for attempt in range(self.max_retries + 1):
            try:
                req = Request(url, data=data, headers=hdrs, method=method)
                with urlopen(req, timeout=self.timeout) as resp:
                    payload = resp.read()
                    if raw:
                        return {
                            "data": payload,
                            "content_type": resp.headers.get("Content-Type"),
                            "filename": _filename_of(resp.headers.get("Content-Disposition")),
                        }
                    return _unwrap(payload)
            except HTTPError as err:
                body_bytes = err.read()
                parsed = _safe_json(body_bytes)
                zerr = _error_from(err.code, parsed, body_bytes, err.headers)
                if err.code >= 500 and attempt < self.max_retries:
                    last_err = zerr
                    time.sleep(2 ** attempt * 0.2)
                    continue
                raise zerr
            except URLError as err:
                last_err = ZataboxError(code="NETWORK_ERROR", message=str(err.reason), status=0)
                if attempt < self.max_retries:
                    time.sleep(2 ** attempt * 0.2)
                    continue
                raise last_err
            except (TimeoutError, OSError) as err:
                last_err = ZataboxError(code="NETWORK_ERROR", message=str(err), status=0)
                if attempt < self.max_retries:
                    time.sleep(2 ** attempt * 0.2)
                    continue
                raise last_err
        raise last_err

    def paginate(self, list_fn, query=None):
        """Yield each page of a cursor list, following either cursor shape."""
        query = dict(query or {})
        cursor = query.get("cursor")
        while True:
            page_query = dict(query)
            if cursor:
                page_query["cursor"] = cursor
            data = list_fn(page_query)
            yield data
            cursor = _next_cursor(data)
            if not cursor:
                break

    @staticmethod
    def verify_webhook(payload, signature_header, secret, tolerance_sec=300):
        """Validate an inbound webhook signature; return the parsed event or raise."""
        if not signature_header:
            raise ZataboxError(code="MISSING_SIGNATURE", message="Signature header is required.")
        if not secret:
            raise ValueError("verify_webhook: pass the endpoint secret")
        parts = {}
        for piece in signature_header.split(","):
            if "=" in piece:
                k, v = piece.split("=", 1)
                parts[k.strip()] = v
        t, sig = parts.get("t"), parts.get("v1")
        if not t or not sig:
            raise ZataboxError(code="INVALID_SIGNATURE", message="Malformed signature header.")
        raw = payload if isinstance(payload, str) else json.dumps(payload)
        expected = hmac.new(
            secret.encode(), ("%s.%s" % (t, raw)).encode(), hashlib.sha256
        ).hexdigest()
        if not hmac.compare_digest(expected, sig):
            raise ZataboxError(code="INVALID_SIGNATURE", message="Signature mismatch.")
        if tolerance_sec > 0:
            try:
                if abs(int(time.time()) - int(t)) > tolerance_sec:
                    raise ZataboxError(code="SIGNATURE_EXPIRED", message="Timestamp outside tolerance.")
            except ValueError:
                pass
        return json.loads(raw) if isinstance(payload, str) else payload


def _safe_json(data):
    try:
        return json.loads(data.decode("utf-8"))
    except Exception:
        return None


def _unwrap(data):
    parsed = _safe_json(data)
    if isinstance(parsed, dict) and "data" in parsed:
        return parsed["data"]
    return parsed


def _filename_of(disposition):
    if not disposition:
        return None
    import re
    m = re.search(r'filename="?([^"]+)"?', disposition)
    return m.group(1) if m else None


def _next_cursor(data):
    if not isinstance(data, dict):
        return None
    pagination = data.get("pagination")
    if isinstance(pagination, dict) and pagination.get("cursor"):
        return pagination["cursor"]
    if data.get("nextCursor"):
        return data["nextCursor"]
    meta = data.get("meta")
    if isinstance(meta, dict) and meta.get("cursor"):
        return meta["cursor"]
    return None


def _error_from(status, parsed, raw, headers):
    err = (parsed or {}).get("error", {}) if isinstance(parsed, dict) else {}
    details = err.get("details")
    if status == 429:
        retry_after = headers.get("Retry-After") if headers else None
        details = dict(details or {})
        if retry_after:
            details["retryAfter"] = int(retry_after)
    code = err.get("code") or ("RATE_LIMITED" if status == 429 else "HTTP_%s" % status)
    request_id = None
    if isinstance(parsed, dict) and isinstance(parsed.get("meta"), dict):
        request_id = parsed["meta"].get("request_id")
    message = err.get("message") or (raw.decode("utf-8", "replace") if raw else "")
    return ZataboxError(code=code, message=message, status=status,
                        request_id=request_id, details=details)
