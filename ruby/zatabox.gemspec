# frozen_string_literal: true

require_relative "lib/zatabox/version"

Gem::Specification.new do |spec|
  spec.name = "zatabox"
  spec.version = Zatabox::VERSION
  spec.authors = ["Zatabox"]
  spec.email = ["developers@zatabox.com"]
  spec.summary = "Official Ruby SDK for the Zatabox Tickets REST API."
  spec.description = "White-label event ticketing as a REST API. Zero gem dependencies (net/http + openssl)."
  spec.homepage = "https://zatabox.com"
  spec.license = "MIT"
  spec.required_ruby_version = ">= 2.5"

  spec.files = Dir["lib/**/*.rb", "README.md", "LICENSE"]
  spec.require_paths = ["lib"]
  spec.metadata["documentation_uri"] = "https://zatabox.com/docs"
  spec.metadata["source_code_uri"] = "https://github.com/mysmartrobot/zataboxtickets/tree/main/ruby"
  spec.metadata["bug_tracker_uri"] = "https://github.com/mysmartrobot/zataboxtickets/issues"
  # Distributed via GitHub, not RubyGems block accidental `gem push`.
  spec.metadata["allowed_push_host"] = "https://github.com/mysmartrobot/zataboxtickets"
end
