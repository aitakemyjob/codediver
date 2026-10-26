namespace Shop;

/// <summary>Keeps orders in memory.</summary>
public class MemoryStore : IStore
{
    private readonly Dictionary<string, Order> _orders = new();

    /// <summary>How many orders are stored.</summary>
    public int Count => _orders.Count;

    /// <summary>Stores an order, replacing one with the same ID.</summary>
    public void Save(Order o)
    {
        _orders[o.Id] = o;
    }

    /// <summary>Loads an order by ID, or null.</summary>
    public Order? Find(string id) => _orders.GetValueOrDefault(id);
}
