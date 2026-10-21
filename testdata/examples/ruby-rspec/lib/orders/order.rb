# The order model: validation and pricing.
module Orders
  # One customer order.
  Order = Struct.new(:id, :sku, :qty, :price_cents, keyword_init: true)

  BULK_MINIMUM = 10

  # Checks an order before it is stored; returns an error message or nil.
  def self.validate(order)
    return 'quantity must be at least 1' if order.qty <= 0
    return 'sku is required' if order.sku.to_s.empty?
    return 'quantity is over the limit of 1000' if order.qty > 1000

    nil
  end

  # What the customer pays, in cents.
  def self.total(order)
    order.qty * unit_price(order)
  end

  # Applies the bulk discount. Only total calls it.
  def self.unit_price(order)
    if order.qty >= BULK_MINIMUM
      order.price_cents * 9 / 10
    else
      order.price_cents
    end
  end
  private_class_method :unit_price

  # The old pricing, kept for a report nobody runs. Nothing calls it.
  def self.legacy_total(order)
    sum = 0
    order.qty.times { sum += order.price_cents }
    sum
  end
end
