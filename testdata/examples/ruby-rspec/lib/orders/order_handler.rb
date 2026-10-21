module Orders
  # Handles order requests: "id=o1;sku=A1;qty=2;price=100".
  class OrderHandler
    def initialize(store)
      @store = store
    end

    # Validates, prices, stores and announces an order; returns a status line.
    def create_order(body)
      order = parse(body)
      return '400 bad request' unless order

      problem = Orders.validate(order)
      return "422 #{problem}" if problem

      @store.save order
      Notifier.deliver(order)
      total = Orders.total(order)
      "201 total=#{total}"
    end

    # Returns one order.
    def get_order(id)
      @store.find(id)&.to_h&.to_s || '404 not found'
    end

    private

    # Reads key=value pairs separated by semicolons; nil if malformed.
    def parse(body)
      fields = {}
      body.split(';').each do |part|
        key, value = part.split('=', 2)
        return nil unless value

        fields[key] = value
      end
      Order.new(id: fields['id'], sku: fields['sku'], qty: fields['qty'].to_i, price_cents: fields['price'].to_i)
    end
  end
end
