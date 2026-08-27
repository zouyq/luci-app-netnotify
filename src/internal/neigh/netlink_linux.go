//go:build linux

package neigh

import (
	"context"
	"fmt"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

// NetlinkWatcher watches IPv4 neighbour updates.
type NetlinkWatcher struct{}

func NewWatcher() Watcher {
	return &NetlinkWatcher{}
}

func (w *NetlinkWatcher) Watch(ctx context.Context, out chan<- Event) error {
	ch := make(chan netlink.NeighUpdate)
	done := make(chan struct{})
	if err := netlink.NeighSubscribe(ch, done); err != nil {
		return fmt.Errorf("neigh subscribe: %w", err)
	}
	defer close(done)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case u, ok := <-ch:
			if !ok {
				return nil
			}
			if ev, ok := neighToEvent(u.Neigh, u.Type == syscall.RTM_DELNEIGH); ok {
				select {
				case out <- ev:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

// Dump lists current IPv4 neighbours.
func (w *NetlinkWatcher) Dump() ([]Event, error) {
	list, err := netlink.NeighList(0, syscall.AF_INET)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(list))
	for _, n := range list {
		if ev, ok := neighToEvent(n, false); ok {
			out = append(out, ev)
		}
	}
	return out, nil
}

func neighToEvent(n netlink.Neigh, deleted bool) (Event, bool) {
	if n.Family != syscall.AF_INET {
		return Event{}, false
	}
	if len(n.HardwareAddr) == 0 && !deleted {
		return Event{}, false
	}
	ifi, _ := net.InterfaceByIndex(n.LinkIndex)
	iface := ""
	if ifi != nil {
		iface = ifi.Name
	}
	ev := Event{
		MAC:     n.HardwareAddr,
		IP:      n.IP,
		Iface:   iface,
		Deleted: deleted,
		State:   mapNUD(n.State),
	}
	return ev, true
}

func mapNUD(state int) NUD {
	switch {
	case state&netlink.NUD_REACHABLE != 0:
		return NUDReachable
	case state&netlink.NUD_STALE != 0:
		return NUDStale
	case state&netlink.NUD_DELAY != 0:
		return NUDDelay
	case state&netlink.NUD_PROBE != 0:
		return NUDProbe
	case state&netlink.NUD_FAILED != 0:
		return NUDFailed
	case state&netlink.NUD_PERMANENT != 0:
		return NUDPermanent
	case state&netlink.NUD_NOARP != 0:
		return NUDNoARP
	case state&netlink.NUD_INCOMPLETE != 0:
		return NUDIncomplete
	default:
		return NUDNone
	}
}
