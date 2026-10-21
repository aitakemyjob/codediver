using System.Net.Http.Json;

namespace Shop;

/// <summary>
/// Tells the warehouse about new orders. It reads the clock, the environment
/// and the network directly, which is what makes it hard to test.
/// </summary>
public static class Notifier
{
    private static readonly HttpClient Http = new();

    /// <summary>Posts an order to the warehouse webhook.</summary>
    public static async Task SendAsync(Order o)
    {
        var url = Environment.GetEnvironmentVariable("WAREHOUSE_URL");
        if (url is null)
        {
            return;
        }
        await Http.PostAsJsonAsync(url, new { o.Id, SentAt = DateTime.UtcNow });
    }
}
