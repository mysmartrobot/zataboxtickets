<?php

namespace Zatabox;

use Zatabox\Resources\Registry;

/**
 * Raised for any non-2xx response (and transport failures).
 *
 * Note: $errorCode holds the stable machine string (e.g. "TICKET_SOLD_OUT");
 * the base \Exception::getCode() is unused for the API code.
 */
class ZataboxError extends \Exception
{
    /** @var string */
    public $errorCode;
    /** @var int */
    public $status;
    /** @var string|null */
    public $requestId;
    /** @var mixed */
    public $details;

    public function __construct($errorCode, $message = '', $status = 0, $requestId = null, $details = null)
    {
        parent::__construct($message !== '' ? $message : (string) $errorCode);
        $this->errorCode = $errorCode ?: 'UNKNOWN_ERROR';
        $this->status = $status;
        $this->requestId = $requestId;
        $this->details = $details;
    }
}

/**
 * Official PHP client for the Zatabox Tickets REST API.
 *
 *   $z = new \Zatabox\Client(['apiKey' => 'vt_live_...']);  // vt_test_ -> sandbox
 *   $events = $z->events->list(['q' => 'jazz', 'limit' => 20]);
 *
 * Zero Composer dependencies uses the bundled curl/json/hash extensions.
 */
#[\AllowDynamicProperties]
class Client
{
    const VERSION = '0.3.0';
    const DEFAULT_LIVE = 'https://api.zatabox.com';
    const DEFAULT_SANDBOX = 'https://sandbox.zatabox.com';

    /** @var string */
    public $baseUrl;
    /** @var string|null */
    private $apiKey;
    /** @var string|null */
    private $bearerToken;
    /** @var int */
    private $timeout;
    /** @var int */
    private $maxRetries;
    /** @var string */
    private $userAgent;

    public function __construct(array $options = [])
    {
        $this->apiKey = isset($options['apiKey']) ? $options['apiKey'] : null;
        $this->bearerToken = isset($options['bearerToken']) ? $options['bearerToken'] : null;
        if (!$this->apiKey && !$this->bearerToken) {
            throw new \InvalidArgumentException('zatabox: pass apiKey or bearerToken');
        }
        $this->baseUrl = self::resolveBaseUrl($this->apiKey, isset($options['baseUrl']) ? $options['baseUrl'] : null);
        $this->timeout = isset($options['timeout']) ? (int) $options['timeout'] : 30;
        $this->maxRetries = isset($options['maxRetries']) ? (int) $options['maxRetries'] : 2;
        $this->userAgent = isset($options['userAgent']) ? $options['userAgent'] : ('zatabox-php/' . self::VERSION);

        // Attach every generated namespace: $this->events, $this->orders, …
        foreach (Registry::MAP as $prop => $cls) {
            $this->{$prop} = new $cls($this);
        }
        // Hand-written extra layered onto the generated webhooks namespace.
        $this->webhooks = new WebhooksResourceExt($this);
    }

    /** RFC 3986 encoding of a single path segment. */
    public static function enc($value)
    {
        return rawurlencode((string) $value);
    }

    public function token()
    {
        return $this->apiKey ?: $this->bearerToken;
    }

    public function setBearerToken($token)
    {
        $this->bearerToken = $token;
        return $this;
    }

    /** Build a fully-qualified URL with an optional query map. */
    public function url($path, $query = null)
    {
        $url = $this->baseUrl . $path;
        $qs = self::buildQuery($query);
        if ($qs !== '') {
            $url .= (strpos($url, '?') === false ? '?' : '&') . $qs;
        }
        return $url;
    }

    /**
     * Core request. Returns the unwrapped `data` for JSON responses, or an
     * array { data, contentType, filename } when $opts['raw'] is true.
     *
     * @param array $opts query|body|raw|idempotencyKey|headers
     * @return mixed
     */
    public function request($method, $path, array $opts = [])
    {
        $query = isset($opts['query']) ? $opts['query'] : null;
        $body = array_key_exists('body', $opts) ? $opts['body'] : null;
        $raw = isset($opts['raw']) ? $opts['raw'] : false;
        $headers = isset($opts['headers']) ? $opts['headers'] : [];
        $idem = isset($opts['idempotencyKey']) ? $opts['idempotencyKey'] : null;

        $url = $this->url($path, $query);
        $hdr = [
            'Accept: ' . ($raw ? '*/*' : 'application/json'),
            'User-Agent: ' . $this->userAgent,
            'Authorization: Bearer ' . $this->token(),
        ];
        foreach ($headers as $k => $v) {
            $hdr[] = $k . ': ' . $v;
        }
        $payload = null;
        if ($body !== null && $method !== 'GET') {
            $payload = json_encode($body);
            $hdr[] = 'Content-Type: application/json';
        }
        $hasIdem = false;
        foreach ($headers as $k => $v) {
            if (strcasecmp($k, 'Idempotency-Key') === 0) {
                $hasIdem = true;
                break;
            }
        }
        if ($method !== 'GET' && !$hasIdem) {
            $hdr[] = 'Idempotency-Key: ' . ($idem ?: self::uuid4());
        }

        $attempt = 0;
        while (true) {
            $ch = curl_init($url);
            curl_setopt_array($ch, [
                CURLOPT_CUSTOMREQUEST => $method,
                CURLOPT_RETURNTRANSFER => true,
                CURLOPT_HEADER => true,
                CURLOPT_HTTPHEADER => $hdr,
                CURLOPT_TIMEOUT => $this->timeout,
                CURLOPT_CONNECTTIMEOUT => $this->timeout,
            ]);
            if ($payload !== null) {
                curl_setopt($ch, CURLOPT_POSTFIELDS, $payload);
            }
            $resp = curl_exec($ch);
            if ($resp === false) {
                $errMsg = curl_error($ch);
                curl_close($ch);
                if ($attempt < $this->maxRetries) {
                    $attempt++;
                    usleep((int) (pow(2, $attempt - 1) * 200000));
                    continue;
                }
                throw new ZataboxError('NETWORK_ERROR', $errMsg, 0);
            }
            $status = (int) curl_getinfo($ch, CURLINFO_RESPONSE_CODE);
            $headerSize = (int) curl_getinfo($ch, CURLINFO_HEADER_SIZE);
            curl_close($ch);
            $rawHeaders = substr($resp, 0, $headerSize);
            $bodyStr = substr($resp, $headerSize);

            if ($status >= 400) {
                $err = $this->errorFrom($status, $bodyStr, $rawHeaders);
                if ($status >= 500 && $attempt < $this->maxRetries) {
                    $attempt++;
                    usleep((int) (pow(2, $attempt - 1) * 200000));
                    continue;
                }
                throw $err;
            }
            if ($raw) {
                return [
                    'data' => $bodyStr,
                    'contentType' => self::headerValue($rawHeaders, 'Content-Type'),
                    'filename' => self::filenameOf(self::headerValue($rawHeaders, 'Content-Disposition')),
                ];
            }
            return self::unwrap($bodyStr);
        }
    }

    /**
     * Yield each page of a cursor list, following either cursor shape.
     *
     *   foreach ($z->paginate([$z->organizer, 'events'], ['limit' => 50]) as $page) { ... }
     *
     * @return \Generator
     */
    public function paginate(callable $listFn, array $query = [])
    {
        $cursor = isset($query['cursor']) ? $query['cursor'] : null;
        while (true) {
            $pageQuery = $query;
            if ($cursor) {
                $pageQuery['cursor'] = $cursor;
            }
            $data = $listFn($pageQuery);
            yield $data;
            $cursor = self::nextCursor($data);
            if (!$cursor) {
                break;
            }
        }
    }

    /** Verify an inbound webhook signature; returns the decoded event or throws. */
    public function verifyWebhook($payload, $signatureHeader, $secret, $toleranceSec = 300)
    {
        if (!$signatureHeader) {
            throw new ZataboxError('MISSING_SIGNATURE', 'Signature header is required.');
        }
        if (!$secret) {
            throw new \InvalidArgumentException('verifyWebhook: pass the endpoint secret');
        }
        $parts = [];
        foreach (explode(',', $signatureHeader) as $piece) {
            $kv = explode('=', $piece, 2);
            if (count($kv) === 2) {
                $parts[trim($kv[0])] = $kv[1];
            }
        }
        $t = isset($parts['t']) ? $parts['t'] : null;
        $v1 = isset($parts['v1']) ? $parts['v1'] : null;
        if (!$t || !$v1) {
            throw new ZataboxError('INVALID_SIGNATURE', 'Malformed signature header.');
        }
        $raw = is_string($payload) ? $payload : json_encode($payload);
        $expected = hash_hmac('sha256', $t . '.' . $raw, $secret);
        if (!hash_equals($expected, $v1)) {
            throw new ZataboxError('INVALID_SIGNATURE', 'Signature mismatch.');
        }
        if ($toleranceSec > 0 && abs(time() - (int) $t) > $toleranceSec) {
            throw new ZataboxError('SIGNATURE_EXPIRED', 'Timestamp outside tolerance.');
        }
        return is_string($payload) ? json_decode($payload, true) : $payload;
    }

    // ── helpers ──────────────────────────────────────────────────────────────
    private static function resolveBaseUrl($apiKey, $baseUrl)
    {
        if ($baseUrl) {
            return rtrim($baseUrl, '/');
        }
        if ($apiKey && (strpos($apiKey, 'vt_test_') === 0 || strpos($apiKey, 'sk_test_') === 0)) {
            return self::DEFAULT_SANDBOX;
        }
        return self::DEFAULT_LIVE;
    }

    private static function buildQuery($query)
    {
        if (!$query) {
            return '';
        }
        $pairs = [];
        foreach ($query as $k => $v) {
            if ($v === null) {
                continue;
            }
            $key = rawurlencode((string) $k);
            if (is_array($v)) {
                foreach ($v as $item) {
                    $pairs[] = $key . '=' . rawurlencode((string) $item);
                }
            } elseif (is_bool($v)) {
                $pairs[] = $key . '=' . ($v ? 'true' : 'false');
            } else {
                $pairs[] = $key . '=' . rawurlencode((string) $v);
            }
        }
        return implode('&', $pairs);
    }

    private static function unwrap($bodyStr)
    {
        if ($bodyStr === '' || $bodyStr === null) {
            return null;
        }
        $parsed = json_decode($bodyStr, true);
        if (is_array($parsed) && array_key_exists('data', $parsed)) {
            return $parsed['data'];
        }
        return $parsed === null ? $bodyStr : $parsed;
    }

    private static function nextCursor($data)
    {
        if (!is_array($data)) {
            return null;
        }
        if (!empty($data['pagination']['cursor'])) {
            return $data['pagination']['cursor'];
        }
        if (!empty($data['nextCursor'])) {
            return $data['nextCursor'];
        }
        if (!empty($data['meta']['cursor'])) {
            return $data['meta']['cursor'];
        }
        return null;
    }

    private function errorFrom($status, $bodyStr, $rawHeaders)
    {
        $parsed = json_decode($bodyStr, true);
        $err = (is_array($parsed) && isset($parsed['error'])) ? $parsed['error'] : [];
        $details = isset($err['details']) ? $err['details'] : null;
        if ($status === 429) {
            $details = is_array($details) ? $details : [];
            $retryAfter = self::headerValue($rawHeaders, 'Retry-After');
            if ($retryAfter !== null) {
                $details['retryAfter'] = (int) $retryAfter;
            }
        }
        $code = isset($err['code']) ? $err['code'] : ($status === 429 ? 'RATE_LIMITED' : 'HTTP_' . $status);
        $requestId = (is_array($parsed) && isset($parsed['meta']['request_id'])) ? $parsed['meta']['request_id'] : null;
        $message = isset($err['message']) ? $err['message'] : $bodyStr;
        return new ZataboxError($code, $message, $status, $requestId, $details);
    }

    private static function headerValue($rawHeaders, $name)
    {
        $value = null;
        foreach (preg_split('/\r?\n/', (string) $rawHeaders) as $line) {
            $pos = strpos($line, ':');
            if ($pos === false) {
                continue;
            }
            if (strcasecmp(trim(substr($line, 0, $pos)), $name) === 0) {
                $value = trim(substr($line, $pos + 1));
            }
        }
        return $value;
    }

    private static function filenameOf($disposition)
    {
        if (!$disposition) {
            return null;
        }
        if (preg_match('/filename="?([^"]+)"?/', $disposition, $m)) {
            return $m[1];
        }
        return null;
    }

    private static function uuid4()
    {
        $data = random_bytes(16);
        $data[6] = chr((ord($data[6]) & 0x0f) | 0x40);
        $data[8] = chr((ord($data[8]) & 0x3f) | 0x80);
        return vsprintf('%s%s-%s-%s-%s-%s%s%s', str_split(bin2hex($data), 4));
    }
}

/** WebhooksResource + the inbound signature verifier. */
class WebhooksResourceExt extends \Zatabox\Resources\WebhooksResource
{
    /** @var Client */
    private $c;

    public function __construct(Client $client)
    {
        parent::__construct($client);
        $this->c = $client;
    }

    /** Verify an inbound webhook signature; returns the decoded event or throws. */
    public function verify($payload, $signatureHeader, $secret, $toleranceSec = 300)
    {
        return $this->c->verifyWebhook($payload, $signatureHeader, $secret, $toleranceSec);
    }
}
