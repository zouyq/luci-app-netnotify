//go:build linux

package probe

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/mdlayher/arp"
)

// ARPProber sends a single ARP request and waits for a reply.
type ARPProber struct{}

func New() Prober {
	return &ARPProber{}
}

func (a *ARPProber) Probe(ctx context.Context, iface string, srcIP, dstIP net.IP, dstMAC net.HardwareAddr) (bool, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		ifis, e2 := net.Interfaces()
		if e2 != nil || len(ifis) == 0 {
			return false, fmt.Errorf("iface %s: %w", iface, err)
		}
		for i := range ifis {
			if ifis[i].Flags&net.FlagUp != 0 && ifis[i].Flags&net.FlagLoopback == 0 {
				ifi = &ifis[i]
				break
			}
		}
		if ifi == nil {
			return false, err
		}
	}

	dstIP = dstIP.To4()
	if dstIP == nil {
		return false, fmt.Errorf("dst not ipv4")
	}
	addr, ok := netip.AddrFromSlice(dstIP)
	if !ok || !addr.Is4() {
		return false, fmt.Errorf("invalid dst ip")
	}

	c, err := arp.Dial(ifi)
	if err != nil {
		return false, err
	}
	defer c.Close()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Second)
	}
	_ = c.SetDeadline(deadline)

	done := make(chan error, 1)
	go func() {
		_, err := c.Resolve(addr)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case err := <-done:
		if err != nil {
			return false, err
		}
		return true, nil
	}
}
