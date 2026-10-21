RSpec.describe Orders::MemoryStore do
  it 'saves and finds an order' do
    store = Orders::MemoryStore.new
    store.save(Orders::Order.new(id: 'o1', sku: 'A1', qty: 2, price_cents: 100))
    expect(store.find('o1').qty).to eq(2)
  end
end
