namespace Shop;

/// <summary>Handles order requests: "id=o1;sku=A1;qty=2;price=100".</summary>
public class OrderHandler
{
    private readonly MemoryStore _store;

    /// <summary>Creates a handler backed by store.</summary>
    public OrderHandler(MemoryStore store)
    {
        _store = store;
    }

    /// <summary>Validates, prices, stores and announces an order; returns a status line.</summary>
    public async Task<string> CreateOrderAsync(string body)
    {
        var o = Parse(body);
        if (o is null)
        {
            return "400 bad request";
        }
        var problem = Orders.Validate(o);
        if (problem is not null)
        {
            return "422 " + problem;
        }
        _store.Save(o);
        await Notifier.SendAsync(o);
        return "201 total=" + Orders.Total(o);
    }

    /// <summary>Returns one order.</summary>
    public string GetOrder(string id) => _store.Find(id)?.ToString() ?? "404 not found";

    /// <summary>Reads key=value pairs separated by semicolons; null if malformed.</summary>
    private static Order? Parse(string body)
    {
        string id = "", sku = "";
        int qty = 0, price = 0;
        foreach (var part in body.Split(';'))
        {
            var kv = part.Split('=', 2);
            if (kv.Length != 2)
            {
                return null;
            }
            switch (kv[0])
            {
                case "id": id = kv[1]; break;
                case "sku": sku = kv[1]; break;
                case "qty": qty = int.Parse(kv[1]); break;
                case "price": price = int.Parse(kv[1]); break;
            }
        }
        return new Order(id, sku, qty, price);
    }
}
