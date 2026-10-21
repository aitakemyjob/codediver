using Xunit;

namespace Shop.Tests;

public class OrdersTests
{
    [Fact]
    public void ValidateRejectsZeroQty()
    {
        Assert.Equal("quantity must be at least 1", Orders.Validate(new Order("o1", "A1", 0, 100)));
    }

    [Theory]
    [InlineData(10, 900)]
    [InlineData(2, 200)]
    public void TotalAppliesBulkDiscount(int qty, int want)
    {
        Assert.Equal(want, Orders.Total(new Order("o1", "A1", qty, 100)));
    }
}
