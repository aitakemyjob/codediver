using Xunit;

namespace Shop.Tests;

public class OrderHandlerTests
{
    [Fact]
    public async Task CreateOrderRejectsMalformedBody()
    {
        var handler = new OrderHandler(new MemoryStore());
        Assert.Equal("400 bad request", await handler.CreateOrderAsync("nonsense"));
    }
}
