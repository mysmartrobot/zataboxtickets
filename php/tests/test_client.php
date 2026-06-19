<?php

// End-to-end tests for the PHP SDK against a local PHP built-in server.
//   php -S 127.0.0.1:8799 tests/server.php &
//   ZBX_TEST_BASE=http://127.0.0.1:8799 php tests/test_client.php

require __DIR__ . '/../src/autoload.php';

use Zatabox\Client;
use Zatabox\ZataboxError;

$base = getenv('ZBX_TEST_BASE') ?: 'http://127.0.0.1:8799';
$fail = 0;
function check($cond, $msg)
{
    global $fail;
    if ($cond) {
        echo "ok: $msg\n";
    } else {
        fwrite(STDERR, "FAIL: $msg\n");
        $fail++;
    }
}

// Base URL routing (no network)
check((new Client(['apiKey' => 'vt_test_x']))->baseUrl === 'https://sandbox.zatabox.com', 'sandbox routing');
check((new Client(['apiKey' => 'vt_live_x']))->baseUrl === 'https://api.zatabox.com', 'live routing');
check((new Client(['apiKey' => 'sk_test_x']))->baseUrl === 'https://sandbox.zatabox.com', 'legacy test routing');
check((new Client(['apiKey' => 'vt_test_x', 'baseUrl' => 'http://localhost:4000']))->baseUrl === 'http://localhost:4000', 'baseUrl override');

// Constructor guard
try {
    new Client([]);
    check(false, 'no-credential should throw');
} catch (\InvalidArgumentException $e) {
    check(true, 'no-credential throws');
}

// Path-segment encoding (deterministic, no network)
check(Client::enc('a/b c') === 'a%2Fb%20c', 'path-segment encoding');

$z = new Client(['apiKey' => 'vt_live_x', 'baseUrl' => $base]);

// Namespaces present
check(is_object($z->events) && method_exists($z->events, 'list'), 'events.list present');
check(method_exists($z->webhooks, 'verify'), 'webhooks.verify present');
check(method_exists($z->media, 'upload'), 'media.upload present');
check(method_exists($z->checkin, 'liveUrl'), 'checkin.liveUrl present');
check(is_object($z->whiteLabel) && is_object($z->savedSearches) && is_object($z->support), 'idiomatic namespaces present');
// Platform administration must NOT be exposed in this public SDK.
check(!property_exists($z, 'admin') && !isset($z->admin), 'admin namespace is NOT exposed');

// GET unwrap + query + auth header
$data = $z->events->list(['limit' => 20, 'category' => 'music']);
check($data['echo_query'] === 'limit=20&category=music', 'query string built in order');
check($data['auth'] === 'Bearer vt_live_x', 'auth header');

// Write: body + auto idempotency + content type
$out = $z->orders->create(['items' => []]);
check(!empty($out['idem']), 'auto idempotency key');
check($out['ct'] === 'application/json', 'content-type json');
check($out['body'] === ['items' => []], 'json body echoed');

// Explicit idempotency
$out2 = $z->orders->create(['items' => []], ['idempotencyKey' => 'fixed']);
check($out2['idem'] === 'fixed', 'explicit idempotency key');

// Error mapping
try {
    $z->orders->get('bad');
    check(false, 'error should throw');
} catch (ZataboxError $e) {
    check($e->errorCode === 'ORDER_NOT_FOUND' && $e->status === 404 && $e->requestId === 'req_9' && $e->details === ['id' => 'x'], 'error mapping');
}

// 429 rate limited
try {
    $z->events->list(['rl' => 1]);
    check(false, '429 should throw');
} catch (ZataboxError $e) {
    check($e->errorCode === 'RATE_LIMITED' && ($e->details['retryAfter'] ?? null) === 30, 'rate limited + retryAfter');
}

// Binary
$pdf = $z->tickets->pdf('5');
check($pdf['data'] === '%PDF' && $pdf['contentType'] === 'application/pdf' && $pdf['filename'] === 't.pdf', 'binary download');

// Pagination
$ids = [];
foreach ($z->paginate([$z->events, 'list'], ['limit' => 10]) as $page) {
    $ids = array_merge($ids, $page['items']);
}
check($ids === [1, 2], 'pagination follows cursor');

// SSE URL (no network)
$z2 = new Client(['apiKey' => 'vt_live_x']);
check($z2->checkin->liveUrl('42') === 'https://api.zatabox.com/api/v1/checkin/event/42/live', 'sse url');

// Webhook verify
$secret = 'whsec';
$t = time();
$raw = json_encode(['type' => 'order.paid']);
$sig = hash_hmac('sha256', $t . '.' . $raw, $secret);
$ev = $z2->webhooks->verify($raw, "t=$t,v1=$sig", $secret);
check($ev['type'] === 'order.paid', 'webhook verify good');
try {
    $z2->webhooks->verify($raw, "t=$t,v1=00", $secret);
    check(false, 'bad sig should throw');
} catch (ZataboxError $e) {
    check($e->errorCode === 'INVALID_SIGNATURE', 'webhook bad signature');
}
try {
    $z2->webhooks->verify($raw, '', $secret);
    check(false, 'missing sig should throw');
} catch (ZataboxError $e) {
    check($e->errorCode === 'MISSING_SIGNATURE', 'webhook missing signature');
}

// Multipart upload round-trip (hand-rolled boundary)
$up = $z->media->upload('PNGDATA', ['filename' => 'cover.png', 'contentType' => 'image/png', 'fields' => ['caption' => 'hi']]);
check($up['filename'] === 'cover.png' && $up['content'] === 'PNGDATA' && $up['field'] === 'hi', 'media upload multipart');
check(strpos($up['ct'], 'multipart/form-data; boundary=') === 0, 'multipart content-type boundary');

// setBearerToken
$z3 = new Client(['bearerToken' => 'jwt1', 'baseUrl' => $base]);
$z3->setBearerToken('jwt2');
$me = $z3->users->me();
check($me['auth'] === 'Bearer jwt2', 'setBearerToken swaps credential');

echo "\n" . ($fail === 0 ? "ALL PHP CHECKS PASSED" : "$fail FAILURE(S)") . "\n";
exit($fail > 0 ? 1 : 0);
