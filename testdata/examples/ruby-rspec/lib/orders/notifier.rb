require 'json'
require 'net/http'
require 'time'

module Orders
  # Tells the warehouse about new orders. It reads the clock, the
  # environment and the network directly, which is what makes it hard to
  # test.
  module Notifier
    # Posts an order to the warehouse webhook.
    def self.deliver(order)
      url = ENV['WAREHOUSE_URL']
      return unless url

      body = "{\"id\":\"#{order.id}\",\"sentAt\":\"#{timestamp}\"}"
      Net::HTTP.post(URI(url), body, 'Content-Type' => 'application/json')
    end

    # The time the warehouse sees, in UTC.
    def self.timestamp
      Time.now.utc.iso8601
    end
  end
end
