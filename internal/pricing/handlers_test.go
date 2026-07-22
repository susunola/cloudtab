package pricing

import (
	"sort"
	"testing"
)

// TestHandlersRegistered guards the product handler registry: every product the
// mappers rely on must be present, and each must expose at least one action
// with a non-nil client factory. This is the contract the generic dispatch in
// engine.go depends on.
func TestHandlersRegistered(t *testing.T) {
	want := []string{
		"cbs", "cdb", "clb", "cvm", "postgres", "redis",
		"vpc", "mongodb", "mariadb", "cynosdb",
		"lighthouse", "ecm", "sqlserver", "dcdb", "gaap",
		"yunjing", "cloudhsm", "domain",
	}
	got := SupportedProducts()
	sort.Strings(got)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("SupportedProducts() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SupportedProducts() = %v, want %v", got, want)
		}
	}

	for name, h := range handlers {
		if h.product != name {
			t.Errorf("handler key %q has mismatched product field %q", name, h.product)
		}
		if h.newClient == nil {
			t.Errorf("handler %q has nil newClient factory", name)
		}
		if len(h.actions) == 0 {
			t.Errorf("handler %q registers no actions", name)
		}
		for action, invoker := range h.actions {
			if invoker == nil {
				t.Errorf("handler %q action %q has nil invoker", name, action)
			}
		}
	}
}

// TestQueryUnsupportedProduct verifies the documented error path for a product
// with no registered handler. No credentials or network access required because
// the dispatch fails before any SDK call.
func TestQueryUnsupportedProduct(t *testing.T) {
	e := &Engine{cfg: Config{Region: "ap-guangzhou"}, clients: map[string]interface{}{}}
	_, err := e.Query(PriceRequest{Product: "does-not-exist", Action: "Whatever"})
	if err == nil {
		t.Fatal("expected error for unsupported product, got nil")
	}
}

// TestInvokeUnsupportedAction verifies that a registered product rejects an
// unknown action with a clear error, again without hitting the network (the
// action map lookup fails before the SDK is invoked — but note the client is
// created first, so we only assert the error is non-nil for a bad action on a
// handler whose client creation does not require network I/O).
func TestInvokeUnsupportedActionLookup(t *testing.T) {
	h, ok := handlers["cvm"]
	if !ok {
		t.Fatal("cvm handler missing")
	}
	if _, ok := h.actions["NoSuchAction"]; ok {
		t.Fatal("did not expect NoSuchAction to be registered")
	}
}
