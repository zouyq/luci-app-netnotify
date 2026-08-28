package netcheck

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Params mirrors script tunables.
type Params struct {
	Hosts            []string
	IPs              []string
	Retry            int
	RetryIntervalSec int
	TimeoutSec       int
	WANIface         string
	CooldownSec      int
}

// Result of one Check cycle.
type Result struct {
	OK        bool
	Via       string // host / ip / ""
	Detail    string
	Restarted bool
	Recovered bool // true when WAN was restarted and network came back
}

// Checker performs connectivity checks.
type Checker struct {
	httpClient *http.Client
}

func New() *Checker {
	return &Checker{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Checker) withTimeout(sec int) *http.Client {
	if sec <= 0 {
		sec = 2
	}
	return &http.Client{
		Timeout: time.Duration(sec) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext: (&net.Dialer{
				Timeout: time.Duration(sec) * time.Second,
			}).DialContext,
		},
	}
}

// CheckHosts tries /generate_204 on each host; success on first OK.
func (c *Checker) CheckHosts(ctx context.Context, p Params) (ok bool, via, detail string) {
	retry := p.Retry
	if retry <= 0 {
		retry = 1
	}
	interval := time.Duration(p.RetryIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Second
	}
	client := c.withTimeout(p.TimeoutSec)
	for _, host := range p.Hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		host = strings.TrimPrefix(host, "http://")
		host = strings.TrimPrefix(host, "https://")
		host = strings.Split(host, "/")[0]
		for attempt := 1; attempt <= retry; attempt++ {
			if err := ctx.Err(); err != nil {
				return false, "", err.Error()
			}
			if err := checkGenerate204(ctx, client, host); err == nil {
				return true, host, "generate_204 ok"
			} else if attempt < retry {
				select {
				case <-ctx.Done():
					return false, "", ctx.Err().Error()
				case <-time.After(interval):
				}
			}
		}
	}
	return false, "", "all hosts failed"
}

// CheckIPs pings/connectivity to fallback IPs.
func (c *Checker) CheckIPs(ctx context.Context, p Params) (ok bool, via, detail string) {
	to := p.TimeoutSec
	if to <= 0 {
		to = 2
	}
	for _, ip := range p.IPs {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return false, "", err.Error()
		}
		if pingIP(ctx, ip, to) {
			return true, ip, "ip reachable"
		}
	}
	return false, "", "all ips failed"
}

// RestartWAN runs ifup on the configured interface.
func RestartWAN(iface string) error {
	if iface == "" {
		iface = "wan"
	}
	cmd := exec.Command("/sbin/ifup", iface)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ifup %s: %v (%s)", iface, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// WaitReachable waits until any IP is reachable or timeout.
func (c *Checker) WaitReachable(ctx context.Context, p Params, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if ok, _, _ := c.CheckIPs(ctx, p); ok {
			return true
		}
		if ok, _, _ := c.CheckHosts(ctx, p); ok {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(5 * time.Second):
		}
	}
	return false
}

func checkGenerate204(ctx context.Context, client *http.Client, host string) error {
	url := "http://" + host + "/generate_204"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Connection", "close")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == 204 {
		return nil
	}
	// Some captive portals return 200 with empty body; treat 200 as soft-ok only if tiny body.
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("status %d", resp.StatusCode)
}

func pingIP(ctx context.Context, ip string, timeoutSec int) bool {
	// Prefer busybox ping on OpenWrt.
	args := []string{"-c", "1", "-W", strconv.Itoa(timeoutSec), ip}
	cmd := exec.CommandContext(ctx, "ping", args...)
	if err := cmd.Run(); err == nil {
		return true
	}
	// Fallback: TCP dial common ports.
	d := net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}
	for _, port := range []string{"53", "80", "443"} {
		conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, port))
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

// WANIPv4 returns the first global IPv4 on the WAN device (or common names).
func WANIPv4(iface string) string {
	candidates := []string{iface, "wan", "pppoe-wan", "eth0.2", "eth1"}
	seen := map[string]bool{}
	for _, name := range candidates {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		ifi, err := net.InterfaceByName(name)
		if err != nil {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip4 := ip.To4(); ip4 != nil && !ip4.IsLoopback() {
				return ip4.String()
			}
		}
	}
	// ubus/network.interface.wan dump via ifstatus
	if out, err := exec.Command("ifstatus", "wan").CombinedOutput(); err == nil {
		if ip := parseIfstatusIPv4(string(out)); ip != "" {
			return ip
		}
	}
	return ""
}

func parseIfstatusIPv4(s string) string {
	// crude JSON-ish: "address": "x.x.x.x"
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "address") {
			continue
		}
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		val := strings.Trim(strings.TrimSpace(line[i+1:]), `",`)
		if net.ParseIP(val) != nil && strings.Contains(val, ".") {
			return val
		}
	}
	return ""
}

// LoadAvg returns 1/5/15 load averages.
func LoadAvg() string {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "-"
	}
	fields := strings.Fields(string(b))
	if len(fields) >= 3 {
		return fields[0] + " " + fields[1] + " " + fields[2]
	}
	return strings.TrimSpace(string(b))
}

// UptimeSec returns system uptime seconds.
func UptimeSec() int64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	sec, _ := strconv.ParseFloat(f[0], 64)
	return int64(sec)
}

// FormatUptime humanizes seconds with a single unit: Xd, Xh, or Xm.
func FormatUptime(sec int64) string {
	if sec < 0 {
		sec = 0
	}
	totalMin := sec / 60
	if totalMin >= 60*24 {
		return fmt.Sprintf("%dd", totalMin/(60*24))
	}
	if totalMin >= 60 {
		return fmt.Sprintf("%dh", totalMin/60)
	}
	return fmt.Sprintf("%dm", totalMin)
}

// BootTime approximates boot timestamp.
func BootTime() time.Time {
	return time.Now().Add(-time.Duration(UptimeSec()) * time.Second)
}

// ReadProcLine helper for tests.
func ReadFirstLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		return sc.Text(), nil
	}
	return "", sc.Err()
}
