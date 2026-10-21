RSpec.describe Orders::OrderHandler do
  it 'rejects a malformed body' do
    handler = Orders::OrderHandler.new(Orders::MemoryStore.new)
    expect(handler.create_order('nonsense')).to eq('400 bad request')
  end
end
