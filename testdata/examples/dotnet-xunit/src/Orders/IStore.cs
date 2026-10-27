namespace Shop;

/// <summary>Where orders are kept; OrderHandler only knows this interface.</summary>
public interface IStore
{
    /// <summary>Stores an order, replacing one with the same ID.</summary>
    void Save(Order o);

    /// <summary>Loads an order by ID, or null.</summary>
    Order? Find(string id);
}
