namespace Shop;

/// <summary>Order validation and pricing.</summary>
public static class Orders
{
    private const int BulkMinimum = 10;

    /// <summary>Checks an order before it is stored; returns an error or null.</summary>
    public static string? Validate(Order o)
    {
        if (o.Qty <= 0)
        {
            return "quantity must be at least 1";
        }
        if (string.IsNullOrEmpty(o.Sku))
        {
            return "sku is required";
        }
        if (o.Qty > 1000)
        {
            return "quantity is over the limit of 1000";
        }
        return null;
    }

    /// <summary>What the customer pays, in cents.</summary>
    public static int Total(Order o) => o.Qty * UnitPrice(o);

    /// <summary>Applies the bulk discount. Only Total calls it.</summary>
    private static int UnitPrice(Order o)
    {
        if (o.Qty >= BulkMinimum)
        {
            return o.PriceCents * 9 / 10;
        }
        return o.PriceCents;
    }

    /// <summary>The old pricing, kept for a report nobody runs. Nothing calls it.</summary>
    public static int LegacyTotal(Order o)
    {
        var sum = 0;
        for (var i = 0; i < o.Qty; i++)
        {
            sum += o.PriceCents;
        }
        return sum;
    }
}
