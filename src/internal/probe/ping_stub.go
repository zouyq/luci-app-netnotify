//go:build !linux

package probe

import (
	"context"
	"net"
)

func PingIP(ctx context.Context, ip net.IP) bool {
	return false
}
