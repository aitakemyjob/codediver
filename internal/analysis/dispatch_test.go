package analysis

import "testing"

// A call through an interface or trait links to its declaration, which
// dispatches to the implementations; the implementing type implements it.
func TestGenericInterfaceDispatch(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"java/Store.java": `package shop;

public interface Store {
    void save(Order o);
    Optional<Order> find(String id) throws IOException;
}
`,
		"java/MemoryStore.java": `package shop;

public class MemoryStore implements Store, AutoCloseable {
    public void save(Order o) {
        put(o);
    }

    public Optional<Order> find(String id) {
        return get(id);
    }
}
`,
		"java/Cache.java": `package shop;

public class Cache {
    public void save(Order o) {
    }
}
`,
		"java/Handler.java": `package shop;

public class Handler {
    private final Store store;

    public Handler(Store store) {
        this.store = store;
    }

    public void create(Order o) {
        store.save(o);
        Cache c = new Cache();
        c.save(o);
    }
}
`,
		"cs/IStore.cs": `namespace Shop;

public interface IStore
{
    void Save(Order o);
    Order? Find(string id);
}
`,
		"cs/MemoryStore.cs": `namespace Shop;

public class MemoryStore : IStore
{
    public void Save(Order o)
    {
        _orders[o.Id] = o;
    }

    public Order? Find(string id) => _orders.GetValueOrDefault(id);
}

public class Audited : Base, IStore
{
    public void Save(Order o) { }
}

public class Base
{
    public Order? Find(string id) => null;
}
`,
		"cs/Handler.cs": `namespace Shop;

public class Handler
{
    private readonly IStore _store;

    public Handler(IStore store)
    {
        _store = store;
    }

    public void Create(Order o)
    {
        _store.Save(o);
    }
}
`,
		"kt/Store.kt": `package shop

interface Store {
    fun save(o: Order)
    fun find(id: String): Order?
}

class MemoryStore(val orders: MutableMap<String, Order>) : Base(), Store {
    override fun save(o: Order) {
        orders[o.id] = o
    }

    override fun find(id: String): Order? = orders[id]
}
`,
		"rs/store.rs": `pub trait Store {
    fn save(&mut self, o: Order);
    fn find(&self, id: &str) -> Option<&Order> {
        None
    }
}

pub struct MemoryStore {
    orders: Vec<Order>,
}

impl crate::store::Store for MemoryStore {
    fn save(&mut self, o: Order) {
        self.orders.push(o);
    }
}

pub struct Handler {
    store: Box<dyn Store>,
}

impl Handler {
    pub fn create(&mut self, o: Order) {
        self.store.save(o);
    }
}

pub fn run(s: &mut dyn Store, o: Order) {
    s.save(o);
}
`,
	})
	ix := Build(root, files, testLang)
	for _, c := range []struct{ from, to string }{
		{"h:java/MemoryStore.java#MemoryStore", "h:java/Store.java#Store"},
		{"h:cs/MemoryStore.cs#MemoryStore", "h:cs/IStore.cs#IStore"},
		{"h:cs/MemoryStore.cs#Audited", "h:cs/IStore.cs#IStore"},
		{"h:rs/store.rs#MemoryStore", "h:rs/store.rs#Store"},
		{"h:kt/Store.kt#MemoryStore", "h:kt/Store.kt#Store"},
	} {
		if !hasEdge(ix, c.from, c.to, EdgeImplement) {
			t.Errorf("%s should implement %s", c.from, c.to)
		}
	}
	for _, c := range []struct{ from, to string }{
		{"h:java/Store.java#Store.save", "h:java/MemoryStore.java#MemoryStore.save"},
		{"h:java/Store.java#Store.find", "h:java/MemoryStore.java#MemoryStore.find"},
		{"h:cs/IStore.cs#IStore.Save", "h:cs/MemoryStore.cs#MemoryStore.Save"},
		{"h:cs/IStore.cs#IStore.Save", "h:cs/MemoryStore.cs#Audited.Save"},
		{"h:cs/IStore.cs#IStore.Find", "h:cs/MemoryStore.cs#Base.Find"}, // inherited from the base class
		{"h:rs/store.rs#Store.save", "h:rs/store.rs#MemoryStore.save"},
		{"h:kt/Store.kt#Store.find", "h:kt/Store.kt#MemoryStore.find"},
	} {
		if !hasEdge(ix, c.from, c.to, EdgeDispatch) {
			t.Errorf("%s should dispatch to %s", c.from, c.to)
		}
	}
	if hasEdge(ix, "h:java/Store.java#Store.save", "h:java/Cache.java#Cache.save", EdgeDispatch) {
		t.Error("Cache doesn't implement Store")
	}
	for _, id := range []string{"h:java/Store.java#Store.save", "h:java/Store.java#Store.find", "h:cs/IStore.cs#IStore.Find", "h:rs/store.rs#Store.save", "h:kt/Store.kt#Store.find"} {
		if s := ix.Symbols[id]; s == nil || !s.Abstract || s.Line != s.EndLine {
			t.Errorf("%s should be an abstract one-line declaration, got %+v", id, s)
		}
	}
	if callees := ix.Callees("h:kt/Store.kt#Store.find"); len(callees) != 1 {
		t.Errorf("a bodiless Kotlin declaration calls nothing, only dispatches: %+v", callees)
	}
	if s := ix.Symbols["h:rs/store.rs#Store.find"]; s == nil || s.Abstract {
		t.Errorf("a trait method with a default body isn't abstract: %+v", s)
	}
	// Calls through the interface reach its declaration, not a same-named
	// method of an unrelated class.
	for _, c := range []struct{ from, to string }{
		{"h:java/Handler.java#Handler.create", "h:java/Store.java#Store.save"},
		{"h:java/Handler.java#Handler.create", "h:java/Cache.java#Cache.save"},
		{"h:cs/Handler.cs#Handler.Create", "h:cs/IStore.cs#IStore.Save"},
		{"h:rs/store.rs#Handler.create", "h:rs/store.rs#Store.save"},
		{"h:rs/store.rs#run", "h:rs/store.rs#Store.save"},
	} {
		if !hasEdge(ix, c.from, c.to, EdgeCall) {
			t.Errorf("%s should call %s; callees %+v", c.from, c.to, ix.Callees(c.from))
		}
	}
	for _, e := range ix.Callees("h:java/Handler.java#Handler.create") {
		if e.Guessed {
			t.Errorf("typed receivers aren't guesses: %+v", e)
		}
	}
}

// Extension functions are qualified by the type they extend, so they
// don't collide with same-named methods.
func TestGenericExtensionFunctions(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"Text.kt": `package shop

fun String.slug(): String = lowercase()

fun <T> List<T>.second(): T = this[1]

fun String?.orBlank(): String = this ?: ""

fun plain(s: String) = s.slug()
`,
		"Text.scala": `package shop

def (s: String).shout: String = s.toUpperCase

extension (s: String)
  def slug: String = s.toLowerCase
  def words: Int = s.split(" ").length

extension (n: Int) def double: Int = n * 2

def plain(s: String): String = s.slug
`,
	})
	ix := Build(root, files, testLang)
	for _, id := range []string{
		"h:Text.kt#String.slug", "h:Text.kt#List.second", "h:Text.kt#String.orBlank", "h:Text.kt#plain",
		"h:Text.scala#String.shout", "h:Text.scala#String.slug", "h:Text.scala#String.words", "h:Text.scala#Int.double", "h:Text.scala#plain",
	} {
		if ix.Symbols[id] == nil {
			var ids []string
			for k := range ix.Symbols {
				ids = append(ids, k)
			}
			t.Errorf("missing %s; have %v", id, ids)
		}
	}
	if s := ix.Symbols["h:Text.kt#String.slug"]; s != nil && (s.Name != "slug" || s.Receiver != "String") {
		t.Errorf("String.slug: name %q receiver %q", s.Name, s.Receiver)
	}
}

func TestTypeList(t *testing.T) {
	for in, want := range map[string]string{
		"Store, AutoCloseable ":       "Store AutoCloseable",
		"Base(), Named<T> by impl ":   "Base Named",
		"IStore<Order> where T : new": "IStore",
		"java.io.Closeable":           "Closeable",
	} {
		got := ""
		for i, s := range typeList(in) {
			if i > 0 {
				got += " "
			}
			got += s
		}
		if got != want {
			t.Errorf("typeList(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRustType(t *testing.T) {
	for in, want := range map[string]string{
		"&mut dyn Store": "Store", "Box<dyn Store>": "Store", "&'a Store": "Store",
		"impl Store": "Store", "Rc<RefCell<MemoryStore>>": "MemoryStore", "crate::store::Store": "crate.store.Store",
		"Vec<Order>": "Vec<Order>",
	} {
		if got := rustType(in); got != want {
			t.Errorf("rustType(%q) = %q, want %q", in, got, want)
		}
	}
}
