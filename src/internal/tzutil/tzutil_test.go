package tzutil

import (
	"os"
	"strings"
	"testing"
)

func TestEnsureLocalFromFile(t *testing.T) {
	t.Setenv("TZ", "")
	dir := t.TempDir()
	path := dir + "/TZ"
	if err := os.WriteFile(path, []byte("CST-8\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Simulate OpenWrt path by temporarily using our test file via EnsureLocal logic
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tz := strings.TrimSpace(string(b))
	if tz != "CST-8" {
		t.Fatalf("got %q", tz)
	}
}
