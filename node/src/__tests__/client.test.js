// Smoke tests for @zatabox/node. Run with: npm test
const test = require('node:test');
const assert = require('node:assert');
const crypto = require('node:crypto');
const { ZataboxClient, Client, ZataboxError, VERSION } = require('../index');

function jsonRes(status, obj, headers = {}) {
  const h = new Map(Object.entries({ 'content-type': 'application/json', ...headers }));
  return Promise.resolve({
    ok: status < 400, status, statusText: '',
    headers: h, text: () => Promise.resolve(JSON.stringify(obj)),
  });
}
function rawRes(status, buf, ct) {
  const h = new Map([['content-type', ct], ['content-disposition', 'inline; filename="t.pdf"']]);
  return Promise.resolve({
    ok: status < 400, status, statusText: '', headers: h,
    arrayBuffer: () => Promise.resolve(buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength)),
    text: () => Promise.resolve(''),
  });
}

test('constructor requires a credential', () => {
  assert.throws(() => new ZataboxClient({}), /apiKey.*bearerToken/);
});

test('base URL routing by key prefix', () => {
  assert.strictEqual(new Client({ apiKey: 'vt_test_x' }).baseUrl, 'https://sandbox.zatabox.com');
  assert.strictEqual(new Client({ apiKey: 'vt_live_x' }).baseUrl, 'https://api.zatabox.com');
  assert.strictEqual(new Client({ apiKey: 'sk_test_x' }).baseUrl, 'https://sandbox.zatabox.com');
  assert.strictEqual(new Client({ apiKey: 'vt_test_x', baseUrl: 'http://localhost:4000' }).baseUrl, 'http://localhost:4000');
});

test('every advertised namespace is present', () => {
  const c = new Client({ apiKey: 'vt_live_x' });
  for (const ns of ['auth', 'users', 'events', 'orders', 'payments', 'checkin', 'organizer',
    'webhooks', 'community', 'growth', 'whiteLabel', 'integrations', 'wallets', 'media']) {
    assert.strictEqual(typeof c[ns], 'object', `missing namespace ${ns}`);
  }
  assert.strictEqual(typeof c.events.list, 'function');
  assert.strictEqual(typeof c.webhooks.verify, 'function');
  assert.strictEqual(typeof c.media.upload, 'function');
  assert.strictEqual(typeof c.checkin.liveUrl, 'function');
});

test('platform administration is NOT exposed', () => {
  // The /api/v1/admin/* surface must never ship in this public SDK.
  const c = new Client({ apiKey: 'vt_live_x' });
  assert.strictEqual(c.admin, undefined, 'admin namespace must not exist');
});

test('GET unwraps the data envelope and builds query', async () => {
  let seen;
  const c = new Client({ apiKey: 'vt_live_x', fetch: (url, init) => { seen = { url, init }; return jsonRes(200, { success: true, data: { items: [1] } }); } });
  const data = await c.events.list({ limit: 20, category: 'music' });
  assert.deepStrictEqual(data, { items: [1] });
  assert.match(seen.url, /\/api\/v1\/events\?limit=20&category=music$/);
  assert.strictEqual(seen.init.headers.Authorization, 'Bearer vt_live_x');
  assert.strictEqual(seen.init.method, 'GET');
});

test('writes send JSON body + auto Idempotency-Key', async () => {
  let seen;
  const c = new Client({ apiKey: 'vt_live_x', fetch: (url, init) => { seen = init; return jsonRes(201, { success: true, data: { id: 'o1' } }); } });
  await c.orders.create({ items: [] });
  assert.strictEqual(seen.headers['Content-Type'], 'application/json');
  assert.ok(seen.headers['Idempotency-Key'], 'auto idempotency key');
  assert.strictEqual(seen.body, JSON.stringify({ items: [] }));
});

test('explicit idempotency key is honoured', async () => {
  let seen;
  const c = new Client({ apiKey: 'vt_live_x', fetch: (url, init) => { seen = init; return jsonRes(201, { success: true, data: {} }); } });
  await c.orders.create({ items: [] }, { idempotencyKey: 'fixed-key' });
  assert.strictEqual(seen.headers['Idempotency-Key'], 'fixed-key');
});

test('path params are encoded', async () => {
  let seen;
  const c = new Client({ apiKey: 'vt_live_x', fetch: (url) => { seen = url; return jsonRes(200, { success: true, data: {} }); } });
  await c.events.get('a/b c');
  assert.match(seen, /\/api\/v1\/events\/a%2Fb%20c$/);
});

test('errors become ZataboxError with code/status/requestId', async () => {
  const c = new Client({ apiKey: 'vt_live_x', maxRetries: 0, fetch: () => jsonRes(404, { success: false, error: { code: 'ORDER_NOT_FOUND', message: 'nope', details: { id: 'x' } }, meta: { request_id: 'req_1' } }) });
  await assert.rejects(() => c.orders.get('x'), (e) => {
    assert.ok(e instanceof ZataboxError);
    assert.strictEqual(e.code, 'ORDER_NOT_FOUND');
    assert.strictEqual(e.status, 404);
    assert.strictEqual(e.requestId, 'req_1');
    assert.deepStrictEqual(e.details, { id: 'x' });
    return true;
  });
});

test('429 surfaces RATE_LIMITED with retryAfter', async () => {
  const c = new Client({ apiKey: 'vt_live_x', maxRetries: 0, fetch: () => jsonRes(429, { success: false, error: {} }, { 'retry-after': '30' }) });
  await assert.rejects(() => c.events.list(), (e) => {
    assert.strictEqual(e.code, 'RATE_LIMITED');
    assert.strictEqual(e.details.retryAfter, 30);
    return true;
  });
});

test('5xx retries then succeeds', async () => {
  let n = 0;
  const c = new Client({ apiKey: 'vt_live_x', maxRetries: 2, fetch: () => { n++; return n < 2 ? jsonRes(503, { success: false, error: { code: 'X' } }) : jsonRes(200, { success: true, data: { ok: true } }); } });
  assert.deepStrictEqual(await c.events.list(), { ok: true });
  assert.strictEqual(n, 2);
});

test('binary endpoints return bytes + content type', async () => {
  const c = new Client({ apiKey: 'vt_live_x', fetch: () => rawRes(200, Buffer.from('%PDF'), 'application/pdf') });
  const r = await c.tickets.pdf('5');
  assert.ok(Buffer.isBuffer(r.data));
  assert.strictEqual(r.contentType, 'application/pdf');
  assert.strictEqual(r.filename, 't.pdf');
});

test('paginate follows both cursor shapes', async () => {
  const c = new Client({ apiKey: 'vt_live_x', fetch: (url) => {
    if (/cursor=c2/.test(url)) return jsonRes(200, { success: true, data: { items: [2], nextCursor: null } });
    return jsonRes(200, { success: true, data: { items: [1], pagination: { cursor: 'c2' } } });
  } });
  const ids = [];
  for await (const p of c.paginate(c.events.list, { limit: 10 })) ids.push(...p.items);
  assert.deepStrictEqual(ids, [1, 2]);
});

test('SSE helper returns a fully-qualified URL', () => {
  const c = new Client({ apiKey: 'vt_live_x' });
  assert.strictEqual(c.checkin.liveUrl('42'), 'https://api.zatabox.com/api/v1/checkin/event/42/live');
});

test('webhook verify accepts a valid signature and rejects a bad one', () => {
  const c = new Client({ apiKey: 'vt_live_x' });
  const secret = 'whsec_test';
  const t = Math.floor(Date.now() / 1000);
  const raw = JSON.stringify({ type: 'order.paid', id: 'evt_1' });
  const sig = crypto.createHmac('sha256', secret).update(`${t}.${raw}`).digest('hex');
  const ev = c.webhooks.verify(raw, `t=${t},v1=${sig}`, secret);
  assert.strictEqual(ev.type, 'order.paid');
  assert.throws(() => c.webhooks.verify(raw, `t=${t},v1=00`, secret), (e) => e.code === 'INVALID_SIGNATURE');
  assert.throws(() => c.webhooks.verify(raw, '', secret), (e) => e.code === 'MISSING_SIGNATURE');
});

test('setBearerToken swaps the credential', async () => {
  let auth;
  const c = new Client({ bearerToken: 'jwt1', fetch: (url, init) => { auth = init.headers.Authorization; return jsonRes(200, { success: true, data: {} }); } });
  c.setBearerToken('jwt2');
  await c.users.me();
  assert.strictEqual(auth, 'Bearer jwt2');
});

test('media.upload builds a multipart form', async () => {
  let body;
  const c = new Client({ apiKey: 'vt_live_x', fetch: (url, init) => { body = init.body; return jsonRes(200, { success: true, data: { ok: true } }); } });
  await c.media.upload(Buffer.from('PNGDATA'), { filename: 'cover.png', contentType: 'image/png', fields: { caption: 'hi' } });
  assert.ok(body instanceof FormData);
  const file = body.get('file');
  assert.strictEqual(file.name, 'cover.png');
  assert.strictEqual(file.type, 'image/png');
  assert.strictEqual(await file.text(), 'PNGDATA');
  assert.strictEqual(body.get('caption'), 'hi');
});

test('VERSION is exported', () => {
  assert.match(VERSION, /^\d+\.\d+\.\d+/);
});
