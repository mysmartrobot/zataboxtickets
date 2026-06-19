# frozen_string_literal: true

# zatabox official Ruby SDK for the Zatabox Tickets REST API.
# Zero gem dependencies (net/http + openssl + json from the standard library).
#
#   require "zatabox"
#   z = Zatabox::Client.new(api_key: "vt_live_...")
#   events = z.events.list(q: "jazz", limit: 20)

require_relative "zatabox/version"
require_relative "zatabox/errors"
require_relative "zatabox/resources"
require_relative "zatabox/client"

module Zatabox
end
