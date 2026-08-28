package tzutil

import (
	"os"
	"strings"
	"time"
)

func init() {
	EnsureLocal()
}

// EnsureLocal sets time.Local from OpenWrt /etc/TZ when TZ env is unset.
// Go on OpenWrt may not pick up /etc/TZ automatically; logs otherwise show UTC.
func EnsureLocal() {
	tz := strings.TrimSpace(os.Getenv("TZ"))
	if tz == "" {
		for _, path := range []string{"/etc/TZ", "/tmp/TZ"} {
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			tz = strings.TrimSpace(string(b))
			if tz != "" {
				_ = os.Setenv("TZ", tz)
				break
			}
		}
	}
	if loc := resolveLocation(tz); loc != nil {
		time.Local = loc
	}
}

func resolveLocation(tz string) *time.Location {
	if tz == "" {
		return nil
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	// Common OpenWrt / China router shorthand.
	switch {
	case strings.Contains(tz, "CST-8"), strings.Contains(tz, "Asia/Shanghai"):
		return time.FixedZone("CST", 8*3600)
	case strings.Contains(tz, "UTC"):
		return time.UTC
	}
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return nil
}
