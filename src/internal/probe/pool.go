package probe

import (
	"context"
	"net"
	"sync"
	"time"
)

// Prober confirms a host is alive via ARP (or stub).
type Prober interface {
	Probe(ctx context.Context, iface string, srcIP, dstIP net.IP, dstMAC net.HardwareAddr) (bool, error)
}

// Pool limits concurrent probes.
type Pool struct {
	prober Prober
	sem    chan struct{}
	mu     sync.Mutex
}

func NewPool(prober Prober, max int) *Pool {
	if max <= 0 {
		max = 2
	}
	if max > 2 {
		max = 2
	}
	return &Pool{
		prober: prober,
		sem:    make(chan struct{}, max),
	}
}

// Do runs a probe with global concurrency limit.
func (p *Pool) Do(ctx context.Context, iface string, srcIP, dstIP net.IP, dstMAC net.HardwareAddr) (bool, error) {
	select {
	case p.sem <- struct{}{}:
		defer func() { <-p.sem }()
	case <-ctx.Done():
		return false, ctx.Err()
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return p.prober.Probe(cctx, iface, srcIP, dstIP, dstMAC)
}
