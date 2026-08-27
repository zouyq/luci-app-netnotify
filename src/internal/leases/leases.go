package leases

import (
	"bufio"
	"os"
	"strings"
	"sync"

	"github.com/zouyq/netnotify/internal/config"
)

// Entry is one dnsmasq dhcp.leases line.
type Entry struct {
	MAC      string
	IP       string
	Hostname string
}

// Reader polls or re-reads dhcp.leases cheaply.
type Reader struct {
	Path string
	mu   sync.Mutex
	seen map[string]Entry
}

func New(path string) *Reader {
	return &Reader{Path: path, seen: make(map[string]Entry)}
}

// Refresh reads the file and returns newly appeared or changed leases.
func (r *Reader) Refresh() ([]Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, err := os.Open(r.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	current := make(map[string]Entry)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		// dnsmasq: <expiry> <mac> <ip> <hostname> <clientid>
		if len(fields) < 4 {
			continue
		}
		mac := config.NormalizeMAC(fields[1])
		e := Entry{MAC: mac, IP: fields[2], Hostname: fields[3]}
		if e.Hostname == "*" {
			e.Hostname = ""
		}
		current[mac] = e
	}
	var news []Entry
	for mac, e := range current {
		prev, ok := r.seen[mac]
		if !ok || prev.IP != e.IP {
			news = append(news, e)
		}
	}
	r.seen = current
	return news, sc.Err()
}

// LookupHostname returns hostname for MAC if known.
func (r *Reader) LookupHostname(mac string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	mac = config.NormalizeMAC(mac)
	if e, ok := r.seen[mac]; ok {
		return e.Hostname
	}
	return ""
}
