# frozen_string_literal: true

module Zatabox
  # Raised for any non-2xx response (and transport failures).
  class Error < StandardError
    attr_reader :code, :status, :request_id, :details

    def initialize(message: nil, code: nil, status: 0, request_id: nil, details: nil)
      @code = code || "UNKNOWN_ERROR"
      @status = status
      @request_id = request_id
      @details = details
      super(message || code || "Zatabox request failed")
    end

    def inspect
      "#<Zatabox::Error code=#{@code.inspect} status=#{@status} request_id=#{@request_id.inspect}>"
    end
  end
end
