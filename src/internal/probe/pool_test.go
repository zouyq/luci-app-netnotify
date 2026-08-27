package probe

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type countingProber struct {
	n atomic.Int32
}

func (c *countingProber) Probe(ctx context.Context, iface string, srcIP, dstIP net.IP, dstMAC net.HardwareAddr) (bool, error) {
	c.n.Add(1)
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(50 * time.Millisecond):
		return true, nil
	}
}

func TestPoolConcurrencyCap(t *testing.T) {
	p := NewPool(&countingProber{}, 2)
	if cap(p.sem) != 2 {
		t.Fatalf("cap=%d", cap(p.sem))
	}
	// over-request should still cap at 2
	p2 := NewPool(&countingProber{}, 8)
	if cap(p2.sem) != 2 {
		t.Fatalf("max parallel must be <=2, got %d", cap(p2.sem))
	}
	ctx := context.Background()
	ok, err := p.Do(ctx, "lo", nil, net.IPv4(1, 2, 3, 4), nil)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
