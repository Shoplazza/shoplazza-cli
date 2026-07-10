package auth

import "testing"

func TestKeyBuilders_Lowercase(t *testing.T) {
	if AccountUATKey("Alice@Co.com") != "account:alice@co.com:uat" {
		t.Fatal("uat key")
	}
	if ProfileStoreKey("Prod-US") != "profile:prod-us:store" {
		t.Fatal("profile key")
	}
}

func TestProfileMeta_RoundtripAndRemove(t *testing.T) {
	dir := t.TempDir()
	if err := SaveProfileMeta(dir, "us", ProfileMeta{StoreID: "100001", ExpiresAt: "2099-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	m, err := LoadProfileMeta(dir, "us")
	if err != nil || m.StoreID != "100001" {
		t.Fatalf("roundtrip: %+v %v", m, err)
	}
	if err := RemoveProfileMeta(dir, "us"); err != nil {
		t.Fatal(err)
	}
	if m, _ = LoadProfileMeta(dir, "us"); m.StoreID != "" {
		t.Fatal("zero value after remove")
	}
}
