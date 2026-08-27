//go:build !linux

package probe

import (
	"context"
	"net"
)

// StubProber is used on non-linux hosts so `go build` / tests work.
type StubProber struct{}

func New() Prober {
	return &StubProber{}
}

func (s *StubProber) Probe(ctx context.Context, iface string, srcIP, dstIP net.IP, dstMAC net.HardwareAddr) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
		return false, nil
	}
}
