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

    # How many orders are stored.
    def count
      @orders.size
    end

    # True when no order is stored yet. count is called without
    # parentheses, as the receiver of zero?.
    def empty?
      count.zero?
    end
  end
end
