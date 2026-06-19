# frozen_string_literal: true

require "net/http"
require "uri"
require "json"
require "openssl"
require "securerandom"

module Zatabox
  DEFAULT_LIVE = "https://api.zatabox.com"
  DEFAULT_SANDBOX = "https://sandbox.zatabox.com"

  # RFC 3986 percent-encoding of a single path segment (unreserved chars only).
  def self.encode(value)
    value.to_s.each_byte.map do |b|
      if (b >= 0x30 && b <= 0x39) || (b >= 0x41 && b <= 0x5A) ||
         (b >= 0x61 && b <= 0x7A) || [0x2D, 0x2E, 0x5F, 0x7E].include?(b)
        b.chr
      else
        format("%%%02X", b)
      end
    end.join
  end

  # Constant-time string comparison.
  def self.secure_compare(a, b)
    return false unless a.bytesize == b.bytesize

    left = a.unpack("C*")
    right = b.unpack("C*")
    result = 0
    left.each_index { |i| result |= left[i] ^ right[i] }
    result.zero?
  end

  # Verify an inbound webhook signature; returns the parsed event or raises.
  def self.verify_webhook(payload, signature_header, secret, tolerance_sec = 300)
    raise Error.new(code: "MISSING_SIGNATURE", message: "Signature header is required.") if signature_header.nil? || signature_header.empty?
    raise ArgumentError, "verify_webhook: pass the endpoint secret" if secret.nil? || secret.empty?

    parts = {}
    signature_header.split(",").each do |piece|
      k, v = piece.split("=", 2)
      parts[k.strip] = v if v
    end
    t = parts["t"]
    sig = parts["v1"]
    raise Error.new(code: "INVALID_SIGNATURE", message: "Malformed signature header.") if t.nil? || sig.nil?

    raw = payload.is_a?(String) ? payload : JSON.generate(payload)
    expected = OpenSSL::HMAC.hexdigest("SHA256", secret, "#{t}.#{raw}")
    raise Error.new(code: "INVALID_SIGNATURE", message: "Signature mismatch.") unless secure_compare(expected, sig)

    if tolerance_sec.positive? && (Time.now.to_i - t.to_i).abs > tolerance_sec
      raise Error.new(code: "SIGNATURE_EXPIRED", message: "Timestamp outside tolerance.")
    end

    payload.is_a?(String) ? JSON.parse(raw) : payload
  end

  # The Zatabox Tickets API client.
  #
  #   z = Zatabox::Client.new(api_key: "vt_live_...")   # vt_test_ → sandbox
  #   z.events.list(q: "jazz", limit: 20)
  class Client
    attr_reader :base_url

    def initialize(api_key: nil, bearer_token: nil, base_url: nil,
                   timeout: 30, max_retries: 2, user_agent: nil)
      raise ArgumentError, "zatabox: pass api_key or bearer_token" if api_key.nil? && bearer_token.nil?

      @api_key = api_key
      @bearer_token = bearer_token
      @base_url = resolve_base_url(api_key, base_url)
      @timeout = timeout
      @max_retries = max_retries
      @user_agent = user_agent || "zatabox-ruby/#{Zatabox::VERSION}"

      attach_resources
    end

    def token
      @api_key || @bearer_token
    end

    def set_bearer_token(value)
      @bearer_token = value
      self
    end

    def url(path, query = nil)
      out = @base_url + path
      qs = encode_query(query)
      out += (out.include?("?") ? "&" : "?") + qs unless qs.empty?
      out
    end

    def request(method, path, query: nil, body: nil, idempotency_key: nil, headers: nil, raw: false)
      uri = URI(url(path, query))
      klass = {
        "GET" => Net::HTTP::Get, "POST" => Net::HTTP::Post, "PUT" => Net::HTTP::Put,
        "PATCH" => Net::HTTP::Patch, "DELETE" => Net::HTTP::Delete
      }.fetch(method)

      attempt = 0
      loop do
        begin
          req = klass.new(uri.request_uri)
          req["Authorization"] = "Bearer #{token}"
          req["User-Agent"] = @user_agent
          req["Accept"] = raw ? "*/*" : "application/json"
          (headers || {}).each { |k, v| req[k.to_s] = v.to_s }
          if !body.nil? && method != "GET"
            req["Content-Type"] = "application/json"
            req.body = JSON.generate(body)
          end
          if method != "GET" && req["Idempotency-Key"].nil?
            req["Idempotency-Key"] = idempotency_key || SecureRandom.uuid
          end

          http = Net::HTTP.new(uri.host, uri.port)
          http.use_ssl = uri.scheme == "https"
          http.open_timeout = @timeout
          http.read_timeout = @timeout
          res = http.request(req)

          status = res.code.to_i
          if status >= 400
            err = error_from(status, res)
            if status >= 500 && attempt < @max_retries
              attempt += 1
              sleep(0.2 * (2**(attempt - 1)))
              next
            end
            raise err
          end

          return { data: res.body, content_type: res["Content-Type"], filename: filename_of(res["Content-Disposition"]) } if raw

          return unwrap(res.body)
        rescue Timeout::Error, IOError, SystemCallError, SocketError, OpenSSL::SSL::SSLError => e
          if attempt < @max_retries
            attempt += 1
            sleep(0.2 * (2**(attempt - 1)))
            next
          end
          raise Error.new(code: "NETWORK_ERROR", message: e.message, status: 0)
        end
      end
    end

    # Yield each page of a cursor list. Pass a callable (e.g. z.events.method(:list)).
    #
    #   z.paginate(z.organizer.method(:events), limit: 50) { |page| ... }
    def paginate(list_fn, query = {})
      return enum_for(:paginate, list_fn, query) unless block_given?

      cursor = query[:cursor]
      loop do
        page_query = query.dup
        page_query[:cursor] = cursor if cursor
        data = list_fn.call(page_query)
        yield data
        cursor = next_cursor(data)
        break unless cursor
      end
    end


    private

    def resolve_base_url(api_key, base_url)
      return base_url.sub(%r{/+\z}, "") if base_url
      return DEFAULT_SANDBOX if api_key && (api_key.start_with?("vt_test_") || api_key.start_with?("sk_test_"))

      DEFAULT_LIVE
    end

    def encode_query(query)
      return "" if query.nil? || query.empty?

      pairs = []
      query.each do |k, v|
        next if v.nil?

        key = Zatabox.encode(k.to_s)
        if v.is_a?(Array)
          v.each { |item| pairs << "#{key}=#{Zatabox.encode(item.to_s)}" }
        else
          pairs << "#{key}=#{Zatabox.encode(v.to_s)}"
        end
      end
      pairs.join("&")
    end

    def unwrap(body)
      return nil if body.nil? || body.empty?

      parsed = JSON.parse(body)
      parsed.is_a?(Hash) && parsed.key?("data") ? parsed["data"] : parsed
    rescue JSON::ParserError
      body
    end

    def next_cursor(data)
      return nil unless data.is_a?(Hash)

      pg = data["pagination"]
      return pg["cursor"] if pg.is_a?(Hash) && pg["cursor"]
      return data["nextCursor"] if data["nextCursor"]

      meta = data["meta"]
      return meta["cursor"] if meta.is_a?(Hash) && meta["cursor"]

      nil
    end

    def filename_of(disposition)
      return nil if disposition.nil?

      m = disposition.match(/filename="?([^"]+)"?/)
      m ? m[1] : nil
    end

    def error_from(status, res)
      parsed = begin
        JSON.parse(res.body)
      rescue StandardError
        nil
      end
      err = parsed.is_a?(Hash) ? (parsed["error"] || {}) : {}
      details = err["details"]
      if status == 429
        details = (details || {}).dup
        retry_after = res["Retry-After"]
        details["retryAfter"] = retry_after.to_i if retry_after
      end
      code = err["code"] || (status == 429 ? "RATE_LIMITED" : "HTTP_#{status}")
      request_id = parsed.is_a?(Hash) && parsed["meta"].is_a?(Hash) ? parsed["meta"]["request_id"] : nil
      Error.new(code: code, message: err["message"] || res.body, status: status,
                request_id: request_id, details: details)
    end

    def attach_resources
      instances = {}
      Zatabox::Resources::REGISTRY.each do |name, klass|
        instances[name] = klass.new(self)
      end
      # Hand-written extra layered onto the generated webhooks namespace.
      instances[:webhooks].define_singleton_method(:verify) do |payload, signature, secret, tolerance = 300|
        Zatabox.verify_webhook(payload, signature, secret, tolerance)
      end
      instances.each do |name, inst|
        define_singleton_method(name) { inst }
      end
    end
  end
end
