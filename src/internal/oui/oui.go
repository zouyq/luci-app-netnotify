package oui

import (
	"bufio"
	"os"
	"strings"
	"sync"
)

// DB is an in-memory MAC OUI → vendor map.
type DB struct {
	mu   sync.RWMutex
	vend map[string]string // 6-hex uppercase prefix → vendor
}

// New empty DB.
func New() *DB {
	return &DB{vend: make(map[string]string)}
}

// LoadFile loads lines of "AABBCC\tVendor_Name" or IEEE "AABBCC     (base 16)		Vendor".
func (d *DB) LoadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	m := make(map[string]string, 8192)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		prefix, name := "", ""
		if i := strings.IndexByte(line, '\t'); i > 0 {
			prefix = strings.ToUpper(strings.TrimSpace(line[:i]))
			name = strings.TrimSpace(line[i+1:])
		} else if strings.Contains(line, "(base 16)") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				prefix = strings.ToUpper(fields[0])
				name = strings.Join(fields[3:], "_")
			}
		} else {
			fields := strings.Fields(line)
			if len(fields) >= 2 && len(fields[0]) == 6 {
				prefix = strings.ToUpper(fields[0])
				name = fields[1]
			}
		}
		prefix = strings.ReplaceAll(prefix, ":", "")
		prefix = strings.ReplaceAll(prefix, "-", "")
		if len(prefix) != 6 || name == "" {
			continue
		}
		m[prefix] = name
	}
	if err := sc.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	d.vend = m
	d.mu.Unlock()
	return nil
}

// Lookup returns vendor for MAC, or empty if unknown.
func (d *DB) Lookup(mac string) string {
	mac = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(mac, ":", ""), "-", ""))
	if len(mac) < 6 {
		return ""
	}
	prefix := mac[:6]
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.vend[prefix]
}

// Len returns number of OUI entries.
func (d *DB) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.vend)
}
