//go:build linux

package probe

import (
	"context"
	"net"
	"os/exec"
	"strconv"
)

// PingIP sends one ICMP echo via busybox ping (OpenWrt).
// Sleeping phones often ignore ARP but still answer ping.
func PingIP(ctx context.Context, ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	timeoutSec := 2
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(timeoutSec), ip4.String())
	return cmd.Run() == nil
}
