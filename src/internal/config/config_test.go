package config

import "testing"

func TestNormalizeMAC(t *testing.T) {
	cases := map[string]string{
		"AA-BB-CC-DD-EE-FF": "aa:bb:cc:dd:ee:ff",
		"aabbccddeeff":      "aa:bb:cc:dd:ee:ff",
		"a:b:c:d:e:f":       "0a:0b:0c:0d:0e:0f",
	}
	for in, want := range cases {
		if got := NormalizeMAC(in); got != want {
			t.Fatalf("%s: got %s want %s", in, got, want)
		}
	}
}

func TestAllowed(t *testing.T) {
	cfg := Defaults()
	cfg.Blacklist = []string{NormalizeMAC("aa:bb:cc:dd:ee:01")}
	if cfg.Allowed("aa:bb:cc:dd:ee:01") {
		t.Fatal("blacklisted should deny")
	}
	cfg.Whitelist = []string{NormalizeMAC("aa:bb:cc:dd:ee:02")}
	if cfg.Allowed("aa:bb:cc:dd:ee:03") {
		t.Fatal("not in whitelist should deny")
	}
	if !cfg.Allowed("aa:bb:cc:dd:ee:02") {
		t.Fatal("whitelisted should allow")
	}
}

func TestResolveName(t *testing.T) {
	cfg := Defaults()
	cfg.Aliases[NormalizeMAC("aa:bb:cc:dd:ee:ff")] = "Phone"
	if cfg.ResolveName("aa:bb:cc:dd:ee:ff", "dhcp-host", nil) != "Phone" {
		t.Fatal("alias first")
	}
	if cfg.ResolveName("11:22:33:44:55:66", "laptop", nil) != "laptop" {
		t.Fatal("dhcp hostname")
	}
	if cfg.ResolveName("11:22:33:44:55:66", "", nil) != "unknown" {
		t.Fatal("unknown fallback")
	}
	oui := func(mac string) string {
		if NormalizeMAC(mac) == NormalizeMAC("11:22:33:44:55:66") {
			return "Apple_Inc"
		}
		return ""
	}
	if cfg.ResolveName("11:22:33:44:55:66", "", oui) != "Apple_Inc" {
		t.Fatal("oui fallback")
	}
}
