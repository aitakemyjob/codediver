namespace Shop;

/// <summary>One customer order.</summary>
public record Order(string Id, string Sku, int Qty, int PriceCents);
