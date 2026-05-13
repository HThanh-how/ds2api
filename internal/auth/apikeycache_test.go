package auth

import (
	"testing"
	"time"

	"ds2api/internal/config"
)

func TestAPIKeyCacheManagedPositiveAndRevoke(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["sk-good"],"accounts":[]}`)
	st := config.LoadStore()
	c := NewAPIKeyCache()
	ok, err := c.ManagedByConfigStore(st, "sk-good")
	if err != nil || !ok {
		t.Fatalf("expected managed, ok=%v err=%v", ok, err)
	}
	ok, err = c.ManagedByConfigStore(st, "sk-good")
	if err != nil || !ok {
		t.Fatalf("second hit expected managed, ok=%v err=%v", ok, err)
	}
	c.RegisterRevokedKey("sk-good")
	_, err = c.ManagedByConfigStore(st, "sk-good")
	if err != ErrAPIKeyRevoked {
		t.Fatalf("expected ErrAPIKeyRevoked, got %v", err)
	}
}

func TestAPIKeyCacheRevalidateRemovesStalePositive(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["sk-a"],"accounts":[]}`)
	st := config.LoadStore()
	c := NewAPIKeyCache()
	if ok, err := c.ManagedByConfigStore(st, "sk-a"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	old := apiKeyRevalidate
	apiKeyRevalidate = 0
	defer func() { apiKeyRevalidate = old }()
	if err := st.Update(func(cc *config.Config) error {
		cc.Keys = nil
		cc.APIKeys = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := c.ManagedByConfigStore(st, "sk-a")
	if err != ErrAPIKeyRevoked {
		t.Fatalf("expected ErrAPIKeyRevoked after store drop, got %v", err)
	}
}

func TestAPIKeyCacheClearRevokedAllowsReuse(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["sk-x"],"accounts":[]}`)
	st := config.LoadStore()
	c := NewAPIKeyCache()
	c.RegisterRevokedKey("sk-x")
	if _, err := c.ManagedByConfigStore(st, "sk-x"); err != ErrAPIKeyRevoked {
		t.Fatal(err)
	}
	c.ClearRevokedKey("sk-x")
	ok, err := c.ManagedByConfigStore(st, "sk-x")
	if err != nil || !ok {
		t.Fatalf("after clear revoked expected managed, ok=%v err=%v", ok, err)
	}
}

func TestAPIKeyCachePassthroughUnknown(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["sk-a"],"accounts":[]}`)
	st := config.LoadStore()
	c := NewAPIKeyCache()
	ok, err := c.ManagedByConfigStore(st, "deepseek-raw-token")
	if err != nil || ok {
		t.Fatalf("expected not managed, ok=%v err=%v", ok, err)
	}
}

func TestAPIKeyCacheRevokedExpiry(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["sk-z"],"accounts":[]}`)
	st := config.LoadStore()
	c := NewAPIKeyCache()
	old := apiKeyRevokedBlock
	apiKeyRevokedBlock = 2 * time.Millisecond
	defer func() { apiKeyRevokedBlock = old }()
	c.RegisterRevokedKey("sk-z")
	time.Sleep(5 * time.Millisecond)
	ok, err := c.ManagedByConfigStore(st, "sk-z")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected managed after revoked TTL")
	}
}
