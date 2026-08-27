package neigh

import (
	"context"
	"net"
)

// NUD states we care about (mirrors linux neighbour).
type NUD uint16

const (
	NUDReachable NUD = 1 << iota
	NUDStale
	NUDDelay
	NUDProbe
	NUDFailed
	NUDPermanent
	NUDNoARP
	NUDNone
	NUDIncomplete
)

// Event is a neighbour change.
type Event struct {
	MAC     net.HardwareAddr
	IP      net.IP
	Iface   string
	State   NUD
	Deleted bool
}

// Watcher delivers neighbour events.
type Watcher interface {
	Watch(ctx context.Context, out chan<- Event) error
	// Dump returns current IPv4 neighbour table snapshot (best-effort).
	Dump() ([]Event, error)
}
