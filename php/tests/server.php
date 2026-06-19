<?php

// Test router for the PHP built-in server. Emulates the Zatabox envelope and
// echoes request details back so the client test can assert on them.
//   php -S 127.0.0.1:8799 tests/server.php

$method = $_SERVER['REQUEST_METHOD'];
$path = parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH);
$query = isset($_SERVER['QUERY_STRING']) ? $_SERVER['QUERY_STRING'] : '';
$headers = function_exists('getallheaders') ? getallheaders() : [];
$auth = null;
$idem = null;
$ct = null;
foreach ($headers as $k => $v) {
    $lk = strtolower($k);
    if ($lk === 'authorization') $auth = $v;
    if ($lk === 'idempotency-key') $idem = $v;
    if ($lk === 'content-type') $ct = $v;
}
$rawBody = file_get_contents('php://input');

function envelope($data, $status = 200)
{
    http_response_code($status);
    header('Content-Type: application/json');
    echo json_encode(['success' => $status < 400, 'data' => $data, 'meta' => ['request_id' => 'req_1']]);
    exit;
}

// 404 error path
if ($path === '/api/v1/orders/bad') {
    http_response_code(404);
    header('Content-Type: application/json');
    echo json_encode(['success' => false, 'error' => ['code' => 'ORDER_NOT_FOUND', 'message' => 'nope', 'details' => ['id' => 'x']], 'meta' => ['request_id' => 'req_9']]);
    exit;
}

// 429 rate limit (events.list with ?rl=1)
if ($path === '/api/v1/events' && isset($_GET['rl'])) {
    http_response_code(429);
    header('Content-Type: application/json');
    header('Retry-After: 30');
    echo json_encode(['success' => false, 'error' => new stdClass()]);
    exit;
}

// Binary download
if ($path === '/api/v1/tickets/5/pdf') {
    header('Content-Type: application/pdf');
    header('Content-Disposition: inline; filename="t.pdf"');
    echo '%PDF';
    exit;
}

// Pagination + GET echo
if ($path === '/api/v1/events') {
    if (strpos($query, 'cursor=c2') !== false) {
        envelope(['items' => [2], 'nextCursor' => null]);
    }
    // first page also carries echo fields for the GET test
    envelope(['items' => [1], 'pagination' => ['cursor' => 'c2'], 'echo_query' => $query, 'auth' => $auth]);
}

// Write echo
if ($path === '/api/v1/orders' && $method === 'POST') {
    envelope(['idem' => $idem, 'ct' => $ct, 'auth' => $auth, 'body' => json_decode($rawBody, true)]);
}

// Multipart upload echo the received file + extra field
if ($path === '/api/v1/media/upload' && $method === 'POST') {
    $f = isset($_FILES['file']) ? $_FILES['file'] : null;
    $content = ($f && is_uploaded_file($f['tmp_name'])) ? file_get_contents($f['tmp_name']) : ($f ? @file_get_contents($f['tmp_name']) : null);
    envelope([
        'filename' => $f['name'] ?? null,
        'type' => $f['type'] ?? null,
        'content' => $content,
        'field' => $_POST['caption'] ?? null,
        'ct' => $ct,
    ]);
}

// users.me echoes the auth header (set-bearer test)
if ($path === '/api/v1/users/me') {
    envelope(['auth' => $auth]);
}

// default
envelope(['method' => $method, 'path' => $path]);
