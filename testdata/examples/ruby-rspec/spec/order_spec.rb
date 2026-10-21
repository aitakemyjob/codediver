RSpec.describe Orders do
  def order(qty)
    Orders::Order.new(id: 'o1', sku: 'A1', qty: qty, price_cents: 100)
  end

  describe '.validate' do
    it 'rejects a zero quantity' do
      expect(Orders.validate(order(0))).to eq('quantity must be at least 1')
    end
  end

  describe '.total' do
    it 'applies the bulk discount' do
      expect(Orders.total(order(10))).to eq(900)
    end
  end
end
