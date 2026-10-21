using Xunit;

namespace Shop.Tests;

public class MemoryStoreTests
{
    [Fact]
    public void SaveAndFind()
    {
        var store = new MemoryStore();
        store.Save(new Order("o1", "A1", 2, 100));
        Assert.Equal(2, store.Find("o1")?.Qty);
    }
}
