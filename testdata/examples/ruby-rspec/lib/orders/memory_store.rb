module Orders
  # Keeps orders in memory.
  class MemoryStore
    def initialize
      @orders = {}
    end

    # Stores an order, replacing one with the same ID.
    def save(order)
      @orders[order.id] = order
    end

    # Loads an order by ID, or nil.
    def find(id)
      @orders[id]
    end
  end
end
