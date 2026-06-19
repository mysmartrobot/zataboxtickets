# frozen_string_literal: true

# Smoke tests for the zatabox Ruby SDK. Run: ruby -Ilib test/test_client.rb
require "minitest/autorun"
require "json"
require "openssl"
require "zatabox"

# Stub Net::HTTP#request so no real network calls happen.
$zatabox_handler = nil
module Net
  class HTTP
    def request(req)
      $zatabox_handler.call(req)
    end
  end
end

FakeResp = Struct.new(:code, :body, :headers) do
  def [](key)
    headers[key]
  end
end

def json_ok(obj, code: 200, headers: {})
  FakeResp.new(code.to_s, JSON.generate(obj), { "Content-Type" => "application/json" }.merge(headers))
end

class TestClient < Minitest::Test
  def test_requires_credential
    assert_raises(ArgumentError) { Zatabox::Client.new }
  end

  def test_base_url_routing
    assert_equal "https://sandbox.zatabox.com", Zatabox::Client.new(api_key: "vt_test_x").base_url
    assert_equal "https://api.zatabox.com", Zatabox::Client.new(api_key: "vt_live_x").base_url
    assert_equal "https://sandbox.zatabox.com", Zatabox::Client.new(api_key: "sk_test_x").base_url
    assert_equal "http://localhost:4000", Zatabox::Client.new(api_key: "vt_test_x", base_url: "http://localhost:4000").base_url
  end

  def test_namespaces_present
    c = Zatabox::Client.new(api_key: "vt_live_x")
    %i[auth users events orders checkin organizer webhooks community growth
       white_label saved_searches public_events media].each do |ns|
      assert_respond_to c, ns
    end
    assert_respond_to c.events, :list
    assert_respond_to c.webhooks, :verify
    assert_respond_to c.media, :upload
    assert_respond_to c.checkin, :live_url
  end

  def test_admin_not_exposed
    # The /api/v1/admin/* surface must never ship in this public SDK.
    c = Zatabox::Client.new(api_key: "vt_live_x")
    refute_respond_to c, :admin, "admin namespace must not be exposed"
  end

  def test_get_unwraps_and_query
    seen = {}
    $zatabox_handler = lambda do |req|
      seen[:path] = req.path
      seen[:method] = req.method
      seen[:auth] = req["Authorization"]
      json_ok({ success: true, data: { items: [1] } })
    end
    c = Zatabox::Client.new(api_key: "vt_live_x")
    data = c.events.list(limit: 20, category: "music")
    assert_equal({ "items" => [1] }, data)
    assert_includes seen[:path], "/api/v1/events?"
    assert_includes seen[:path], "limit=20"
    assert_equal "GET", seen[:method]
    assert_equal "Bearer vt_live_x", seen[:auth]
  end

  def test_write_body_and_idempotency
    seen = {}
    $zatabox_handler = lambda do |req|
      seen[:body] = req.body
      seen[:idem] = req["Idempotency-Key"]
      seen[:ct] = req["Content-Type"]
      json_ok({ success: true, data: { id: "o1" } }, code: 201)
    end
    out = Zatabox::Client.new(api_key: "vt_live_x").orders.create(items: [])
    assert_equal({ "id" => "o1" }, out)
    assert_equal({ "items" => [] }, JSON.parse(seen[:body]))
    refute_nil seen[:idem]
    assert_equal "application/json", seen[:ct]
  end

  def test_explicit_idempotency
    seen = {}
    $zatabox_handler = lambda do |req|
      seen[:idem] = req["Idempotency-Key"]
      json_ok({ success: true, data: {} }, code: 201)
    end
    Zatabox::Client.new(api_key: "vt_live_x").orders.create({ items: [] }, idempotency_key: "fixed")
    assert_equal "fixed", seen[:idem]
  end

  def test_path_params_encoded
    seen = {}
    $zatabox_handler = lambda do |req|
      seen[:path] = req.path
      json_ok({ success: true, data: {} })
    end
    Zatabox::Client.new(api_key: "vt_live_x").events.get("a/b c")
    assert_includes seen[:path], "/api/v1/events/a%2Fb%20c"
  end

  def test_error_raises
    $zatabox_handler = lambda do |_req|
      json_ok({ success: false, error: { code: "ORDER_NOT_FOUND", message: "nope", details: { id: "x" } }, meta: { request_id: "req_1" } }, code: 404)
    end
    c = Zatabox::Client.new(api_key: "vt_live_x", max_retries: 0)
    err = assert_raises(Zatabox::Error) { c.orders.get("x") }
    assert_equal "ORDER_NOT_FOUND", err.code
    assert_equal 404, err.status
    assert_equal "req_1", err.request_id
    assert_equal({ "id" => "x" }, err.details)
  end

  def test_429
    $zatabox_handler = lambda do |_req|
      json_ok({ success: false, error: {} }, code: 429, headers: { "Retry-After" => "30" })
    end
    err = assert_raises(Zatabox::Error) { Zatabox::Client.new(api_key: "vt_live_x", max_retries: 0).events.list }
    assert_equal "RATE_LIMITED", err.code
    assert_equal 30, err.details["retryAfter"]
  end

  def test_binary
    $zatabox_handler = lambda do |_req|
      FakeResp.new("200", "%PDF", { "Content-Type" => "application/pdf", "Content-Disposition" => 'inline; filename="t.pdf"' })
    end
    r = Zatabox::Client.new(api_key: "vt_live_x").tickets.pdf("5")
    assert_equal "%PDF", r[:data]
    assert_equal "application/pdf", r[:content_type]
    assert_equal "t.pdf", r[:filename]
  end

  def test_paginate
    $zatabox_handler = lambda do |req|
      if req.path.include?("cursor=c2")
        json_ok({ success: true, data: { items: [2], nextCursor: nil } })
      else
        json_ok({ success: true, data: { items: [1], pagination: { cursor: "c2" } } })
      end
    end
    c = Zatabox::Client.new(api_key: "vt_live_x")
    ids = []
    c.paginate(c.events.method(:list), limit: 10) { |page| ids.concat(page["items"]) }
    assert_equal [1, 2], ids
  end

  def test_sse_url
    c = Zatabox::Client.new(api_key: "vt_live_x")
    assert_equal "https://api.zatabox.com/api/v1/checkin/event/42/live", c.checkin.live_url("42")
  end

  def test_webhook_verify
    c = Zatabox::Client.new(api_key: "vt_live_x")
    secret = "whsec"
    t = Time.now.to_i
    raw = JSON.generate({ type: "order.paid" })
    sig = OpenSSL::HMAC.hexdigest("SHA256", secret, "#{t}.#{raw}")
    ev = c.webhooks.verify(raw, "t=#{t},v1=#{sig}", secret)
    assert_equal "order.paid", ev["type"]
    assert_raises(Zatabox::Error) { c.webhooks.verify(raw, "t=#{t},v1=00", secret) }
    assert_raises(Zatabox::Error) { c.webhooks.verify(raw, "", secret) }
  end

  def test_set_bearer_token
    seen = {}
    $zatabox_handler = lambda do |req|
      seen[:auth] = req["Authorization"]
      json_ok({ success: true, data: {} })
    end
    c = Zatabox::Client.new(bearer_token: "jwt1")
    c.set_bearer_token("jwt2")
    c.users.me
    assert_equal "Bearer jwt2", seen[:auth]
  end

  def test_upload_multipart
    captured = {}
    $zatabox_handler = lambda do |req|
      captured[:body] = req.body
      captured[:ct] = req["Content-Type"]
      json_ok({ success: true, data: { ok: true } })
    end
    Zatabox::Client.new(api_key: "vt_live_x").media.upload(
      "PNGDATA", filename: "cover.png", content_type: "image/png", fields: { caption: "hi" }
    )
    assert captured[:ct].start_with?("multipart/form-data; boundary=")
    assert_includes captured[:body], 'name="file"; filename="cover.png"'
    assert_includes captured[:body], "Content-Type: image/png"
    assert_includes captured[:body], "PNGDATA"
    assert_includes captured[:body], 'name="caption"'
  end

  def test_version
    assert_match(/\A\d+\.\d+\.\d+/, Zatabox::VERSION)
  end
end
