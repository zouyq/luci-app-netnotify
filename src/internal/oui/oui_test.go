package oui

import "testing"

func TestLookupNormalize(t *testing.T) {
	d := New()
	d.mu.Lock()
	d.vend = map[string]string{"AABBCC": "Test_Vendor"}
	d.mu.Unlock()
	if got := d.Lookup("aa:bb:cc:11:22:33"); got != "Test_Vendor" {
		t.Fatalf("got %q", got)
	}
	if d.Lookup("11:22:33:44:55:66") != "" {
		t.Fatal("expected miss")
	}
}
