package account

import (
	"os"
	"testing"
)

func TestHardcoverClientSharesThrottleUntilTokenChanges(t *testing.T) {
	t.Setenv("HARDCOVER_TOKEN", "test-token")
	s := DefaultStore(t.TempDir())
	first := s.HardcoverClient()
	if first == nil || s.HardcoverClient() != first {
		t.Fatal("client isn't shared")
	}
	t.Setenv("HARDCOVER_TOKEN", "new-token")
	if next := s.HardcoverClient(); next == nil || next == first {
		t.Fatal("token change reused old client")
	}
}

func TestHardcoverOnlyCredentialsDoNotEnableCatalog(t *testing.T) {
	t.Setenv("SHELFCHECK_CARD", "")
	s := DefaultStore(t.TempDir())
	if err := os.WriteFile(s.Credentials, []byte("HARDCOVER_TOKEN=test-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if s.HasCatalog() {
		t.Fatal("Hardcover-only credentials enabled catalog login")
	}
	if err := os.WriteFile(s.Credentials, []byte("SHELFCHECK_CARD=test-card\nSHELFCHECK_PIN=test-pin\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !s.HasCatalog() {
		t.Fatal("catalog credentials not detected")
	}
}
