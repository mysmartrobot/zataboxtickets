'use strict';

// @zatabox/node official Node.js SDK for the Zatabox Tickets REST API.
//
// Zero runtime dependencies: global `fetch` (Node 18+) and `node:crypto` only.
//
// The per-endpoint methods live in ./resources.generated.js, emitted from
// ../../spec/endpoints.json by scripts/generate.mjs so this SDK can never drift
// from the API. This file is the hand-written core: transport, auth, retries,
// the error type, pagination, webhook verification and multipart upload.
//
// See ../../spec/CONVENTIONS.md for the cross-language contract.

const crypto = require('node:crypto');
const { attachResources } = require('./resources.generated');

const VERSION = '0.3.0';

const DEFAULT_LIVE = 'https://api.zatabox.com';
const DEFAULT_SANDBOX = 'https://sandbox.zatabox.com';

class ZataboxError extends Error {
  constructor({ code, message, status, requestId, details }) {
    super(message || code || 'Zatabox request failed');
    this.name = 'ZataboxError';
    this.code = code || 'UNKNOWN_ERROR';
    this.status = status ?? 0;
    this.requestId = requestId || null;
    this.details = details;
  }
}

function resolveBaseUrl({ apiKey, baseUrl }) {
  if (baseUrl) return baseUrl.replace(/\/+$/, '');
  if (apiKey && (apiKey.startsWith('vt_test_') || apiKey.startsWith('sk_test_'))) return DEFAULT_SANDBOX;
  return DEFAULT_LIVE;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

class ZataboxClient {
  /**
   * @param {object} options
   * @param {string} [options.apiKey]       vt_live_… / vt_test_… (routes base URL)
   * @param {string} [options.bearerToken]  portal JWT or MCP token
   * @param {string} [options.baseUrl]      explicit override (wins over key prefix)
   * @param {function} [options.fetch]      fetch impl (defaults to global fetch)
   * @param {number} [options.timeoutMs]    per-request timeout (default 30000)
   * @param {number} [options.maxRetries]   transient-failure retries (default 2)
   */
  constructor(options = {}) {
    if (!options.apiKey && !options.bearerToken) {
      throw new Error('@zatabox/node: pass either { apiKey } or { bearerToken }.');
    }
    this.apiKey = options.apiKey || null;
    this._bearerToken = options.bearerToken || null;
    this.baseUrl = resolveBaseUrl(options);
    this.fetch = options.fetch || globalThis.fetch;
    this.timeoutMs = options.timeoutMs ?? 30_000;
    this.maxRetries = options.maxRetries ?? 2;
    this.userAgent = options.userAgent || `zatabox-node/${VERSION}`;
    if (!this.fetch) {
      throw new Error('@zatabox/node: no fetch available use Node 18+ or pass { fetch }.');
    }

    // Generated resource namespaces (client.events, client.orders, …).
    attachResources(this);

    // Hand-written extra layered onto the generated webhooks namespace.
    this.webhooks.verify = (payload, signatureHeader, secret) =>
      verifyWebhook(payload, signatureHeader, secret);
  }

  /** Swap the bearer token at runtime (e.g. after auth.refresh()). */
  setBearerToken(token) {
    this._bearerToken = token;
    return this;
  }

  get token() {
    return this.apiKey || this._bearerToken;
  }

  /** Build a fully-qualified URL with an optional query map. */
  _url(path, query) {
    let url = this.baseUrl + path;
    if (query) {
      const sp = new URLSearchParams();
      for (const [k, v] of Object.entries(query)) {
        if (v === undefined || v === null) continue;
        if (Array.isArray(v)) v.forEach((x) => sp.append(k, String(x)));
        else sp.append(k, String(v));
      }
      const qs = sp.toString();
      if (qs) url += (url.includes('?') ? '&' : '?') + qs;
    }
    return url;
  }

  /**
   * Core request. Returns the unwrapped `data` for JSON responses, or a
   * { data: Buffer, contentType, filename } object when `raw` is set.
   */
  async request(method, path, { query, body, idempotencyKey, headers, raw } = {}) {
    const url = this._url(path, query);
    const hdrs = {
      Accept: raw ? '*/*' : 'application/json',
      'User-Agent': this.userAgent,
      Authorization: `Bearer ${this.token}`,
      ...(headers || {}),
    };
    const hasBody = body !== undefined && method !== 'GET';
    if (hasBody) hdrs['Content-Type'] = 'application/json';
    if (method !== 'GET' && !('Idempotency-Key' in hdrs)) {
      hdrs['Idempotency-Key'] = idempotencyKey || crypto.randomUUID();
    }

    let lastErr = null;
    for (let attempt = 0; attempt <= this.maxRetries; attempt++) {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), this.timeoutMs);
      try {
        const res = await this.fetch(url, {
          method,
          headers: hdrs,
          body: hasBody ? JSON.stringify(body) : undefined,
          signal: controller.signal,
        });
        clearTimeout(timer);

        if (raw && res.ok) {
          const buf = Buffer.from(await res.arrayBuffer());
          return { data: buf, contentType: res.headers.get('content-type'), filename: filenameOf(res) };
        }

        const text = await res.text();
        const json = text ? safeJson(text) : null;
        if (!res.ok) {
          const err = errorFrom(res, json, text);
          if (res.status >= 500 && attempt < this.maxRetries) {
            lastErr = err;
            await sleep(2 ** attempt * 200);
            continue;
          }
          throw err;
        }
        return json && typeof json === 'object' && 'data' in json ? json.data : json;
      } catch (err) {
        clearTimeout(timer);
        if (err instanceof ZataboxError) throw err;
        // Network / abort → retryable.
        if (attempt < this.maxRetries) {
          lastErr = err;
          await sleep(2 ** attempt * 200);
          continue;
        }
        throw new ZataboxError({ code: 'NETWORK_ERROR', message: err.message, status: 0 });
      }
    }
    throw lastErr;
  }

  /**
   * Auto-paginate a cursor list. Yields each page's `data`. Works with both
   * pagination shapes the API uses ({ pagination: { cursor } } | { nextCursor }).
   *
   *   for await (const page of client.paginate(client.organizer.events)) { … }
   */
  async *paginate(listFn, query = {}) {
    let cursor = query.cursor;
    do {
      const data = await listFn({ ...query, ...(cursor ? { cursor } : {}) });
      yield data;
      cursor = nextCursorOf(data);
    } while (cursor);
  }
}

// ── helpers ──────────────────────────────────────────────────────────────────
function safeJson(text) {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function filenameOf(res) {
  const cd = res.headers.get('content-disposition') || '';
  const m = cd.match(/filename="?([^"]+)"?/);
  return m ? m[1] : null;
}

function errorFrom(res, json, text) {
  const e = (json && json.error) || {};
  let details = e.details;
  if (res.status === 429) {
    const ra = res.headers.get('retry-after');
    details = { ...(details || {}), retryAfter: ra ? Number(ra) : undefined };
  }
  return new ZataboxError({
    code: e.code || (res.status === 429 ? 'RATE_LIMITED' : `HTTP_${res.status}`),
    message: e.message || text || res.statusText,
    status: res.status,
    requestId: json && json.meta ? json.meta.request_id : null,
    details,
  });
}

function nextCursorOf(data) {
  if (!data || typeof data !== 'object') return null;
  if (data.pagination && data.pagination.cursor) return data.pagination.cursor;
  if (data.nextCursor) return data.nextCursor;
  if (data.meta && data.meta.cursor) return data.meta.cursor;
  return null;
}

/**
 * Verify an inbound webhook signature. Header scheme: `t=<unix>,v1=<hex-hmac>`;
 * signed string is `<t>.<rawBody>`, HMAC-SHA256 with the endpoint secret.
 * Returns the parsed event on success; throws ZataboxError otherwise.
 */
function verifyWebhook(payload, signatureHeader, secret, toleranceSec = 300) {
  if (!signatureHeader) {
    throw new ZataboxError({ code: 'MISSING_SIGNATURE', message: 'Signature header is required.' });
  }
  if (!secret) throw new Error('webhooks.verify: pass the endpoint secret as the third argument.');
  const parts = Object.fromEntries(
    signatureHeader.split(',').map((p) => {
      const i = p.indexOf('=');
      return [p.slice(0, i).trim(), p.slice(i + 1)];
    })
  );
  const t = parts.t;
  const sig = parts.v1;
  if (!t || !sig) throw new ZataboxError({ code: 'INVALID_SIGNATURE', message: 'Malformed signature header.' });
  const raw = typeof payload === 'string' ? payload : JSON.stringify(payload);
  const expected = crypto.createHmac('sha256', secret).update(`${t}.${raw}`).digest('hex');
  const a = Buffer.from(expected, 'hex');
  const b = Buffer.from(sig, 'hex');
  if (a.length !== b.length || !crypto.timingSafeEqual(a, b)) {
    throw new ZataboxError({ code: 'INVALID_SIGNATURE', message: 'Signature mismatch.' });
  }
  if (toleranceSec > 0) {
    const age = Math.abs(Math.floor(Date.now() / 1000) - Number(t));
    if (Number.isFinite(age) && age > toleranceSec) {
      throw new ZataboxError({ code: 'SIGNATURE_EXPIRED', message: 'Signature timestamp outside tolerance.' });
    }
  }
  return typeof payload === 'string' ? JSON.parse(payload) : payload;
}

// Backwards-compatible alias for the pre-0.3 class name.
const Client = ZataboxClient;

module.exports = { ZataboxClient, Client, ZataboxError, VERSION };
