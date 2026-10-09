package account

import "testing"

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
