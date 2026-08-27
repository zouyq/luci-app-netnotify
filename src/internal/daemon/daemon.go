package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/zouyq/netnotify/internal/config"
	"github.com/zouyq/netnotify/internal/device"
	"github.com/zouyq/netnotify/internal/leases"
	"github.com/zouyq/netnotify/internal/neigh"
	"github.com/zouyq/netnotify/internal/nlog"
	"github.com/zouyq/netnotify/internal/notify"
	"github.com/zouyq/netnotify/internal/oui"
	"github.com/zouyq/netnotify/internal/probe"
)

// Daemon runs the event loop.
type Daemon struct {
	cfg    config.Config
	log    *nlog.Logger
	store  *device.Store
	leases *leases.Reader
	pool   *probe.Pool
	sender notify.Sender
	ouiDB  *oui.DB
	params device.Params

	probeMu sync.Mutex
	pending map[string]time.Time // mac -> next probe not before

	cronMu      sync.Mutex
	lastCronKey string
	lastCronAt  time.Time
}

func New(cfg config.Config, log *nlog.Logger) (*Daemon, error) {
	sender, err := notify.FromConfig(cfg)
	if err != nil {
		return nil, err
	}
	db := oui.New()
	if cfg.OUIEnable {
		if err := db.LoadFile(cfg.OUIPath); err != nil {
			log.Infof("oui db not loaded (%s): %v", cfg.OUIPath, err)
		} else {
			log.Infof("oui db loaded: %d vendors from %s", db.Len(), cfg.OUIPath)
		}
	}
	return &Daemon{
		cfg:    cfg,
		log:    log,
		store:  device.NewStore(),
		leases: leases.New(cfg.DHCPLeasesPath),
		pool:   probe.NewPool(probe.New(), cfg.ProbeMaxParallel),
		sender: sender,
		ouiDB:  db,
		params: device.Params{
			OfflineFailCount:  cfg.OfflineFailCount,
			SuspectTimeoutSec: cfg.SuspectTimeoutSec,
		},
		pending: make(map[string]time.Time),
	}, nil
}

func (d *Daemon) resolveName(mac, dhcpHost string) string {
	var lookup func(string) string
	if d.cfg.OUIEnable && d.ouiDB != nil {
		lookup = d.ouiDB.Lookup
	}
	return d.cfg.ResolveName(mac, dhcpHost, lookup)
}

// Run starts watchers until ctx cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	runtime.GOMAXPROCS(2)
	d.log.Infof("netnotifyd %s starting (%s)", config.Version, d.cfg.String())

	_ = os.MkdirAll(filepath.Dir(d.cfg.StateFile), 0755)
	d.loadState()

	evCh := make(chan neigh.Event, 64)
	w := neigh.NewWatcher()
	// Load DHCP names before neighbour dump so first online events get hostnames.
	d.pollLeases(ctx)
	if snap, err := w.Dump(); err == nil {
		for _, ev := range snap {
			d.handleNeigh(ctx, ev)
		}
	} else {
		d.log.Debugf("neigh dump: %v", err)
	}
	go func() {
		if err := w.Watch(ctx, evCh); err != nil && ctx.Err() == nil {
			d.log.Errorf("neigh watch: %v", err)
		}
	}()

	leaseTick := time.NewTicker(time.Duration(d.cfg.LeasePollSec) * time.Second)
	defer leaseTick.Stop()
	stateTick := time.NewTicker(5 * time.Second)
	defer stateTick.Stop()
	suspectTick := time.NewTicker(5 * time.Second)
	defer suspectTick.Stop()
	cronTick := time.NewTicker(30 * time.Second)
	defer cronTick.Stop()

	for {
		select {
		case <-ctx.Done():
			d.writeState()
			return ctx.Err()
		case ev := <-evCh:
			d.handleNeigh(ctx, ev)
		case <-leaseTick.C:
			d.pollLeases(ctx)
		case <-stateTick.C:
			d.writeState()
			d.trimLog()
		case <-suspectTick.C:
			d.tickSuspect(ctx)
		case <-cronTick.C:
			d.tickCron(ctx)
		}
	}
}

func (d *Daemon) handleNeigh(ctx context.Context, ev neigh.Event) {
	if len(ev.MAC) == 0 {
		return
	}
	// Ignore broadcast / multicast neighbours (NOARP rows, SSDP, etc.)
	if ev.MAC[0]&0x01 != 0 {
		return
	}
	if ev.State == neigh.NUDNoARP {
		return
	}
	if ev.IP != nil {
		if ev.IP.IsMulticast() || ev.IP.IsUnspecified() || ev.IP.Equal(net.IPv4bcast) {
			return
		}
		if ip4 := ev.IP.To4(); ip4 != nil && ip4[3] == 255 {
			return
		}
	}
	if skipIface(ev.Iface) || !d.ifaceAllowed(ev.Iface) {
		return
	}
	mac := config.NormalizeMAC(ev.MAC.String())
	if !d.cfg.Allowed(mac) {
		return
	}
	now := time.Now()
	host := d.leases.LookupHostname(mac)
	name := d.resolveName(mac, host)

	var kind device.EventKind
	switch {
	case ev.Deleted || ev.State == neigh.NUDFailed:
		kind = device.EventFailed
	case ev.State == neigh.NUDReachable || ev.State == neigh.NUDPermanent:
		kind = device.EventStrongUp
	case ev.State == neigh.NUDStale || ev.State == neigh.NUDDelay || ev.State == neigh.NUDProbe:
		kind = device.EventWeakSeen
	case ev.State == neigh.NUDIncomplete:
		kind = device.EventWeakSeen
	default:
		kind = device.EventWeakSeen
	}

	d.store.Lock()
	dev := d.store.GetOrCreateUnsafe(mac)
	if ev.IP != nil {
		dev.IP = ev.IP.String()
	}
	if ev.Iface != "" {
		dev.Iface = ev.Iface
	}
	dev.Name = name
	tr := device.ApplyEvent(dev, kind, d.params, now)
	needProbe := tr.NeedProbe
	becameOnline := tr.BecameOnline
	becameOffline := tr.BecameOffline
	ip, iface, dname := dev.IP, dev.Iface, dev.Name
	state := dev.State
	d.store.Unlock()

	d.log.Debugf("neigh mac=%s ip=%s iface=%s nud=%v -> %s", mac, ip, iface, ev.State, state)

	if becameOnline {
		d.push(ctx, "上线", mac, ip, dname, iface)
	}
	if becameOffline {
		d.push(ctx, "下线", mac, ip, dname, iface)
	}
	if needProbe && (state == device.StatePendingUp || state == device.StateSuspect) {
		d.scheduleProbe(ctx, mac)
	}
}

func (d *Daemon) pollLeases(ctx context.Context) {
	news, err := d.leases.Refresh()
	if err != nil {
		d.log.Debugf("leases: %v", err)
		return
	}
	for _, e := range news {
		mac := config.NormalizeMAC(e.MAC)
		if !d.cfg.Allowed(mac) {
			continue
		}
		name := d.resolveName(mac, e.Hostname)
		var needProbe bool
		d.store.Update(mac, func(dev *device.Device) {
			dev.IP = e.IP
			dev.Name = name
			switch dev.State {
			case device.StateOnline:
				// refresh presence soft hint
				_ = device.ApplyEvent(dev, device.EventStrongUp, d.params, time.Now())
			case device.StateSuspect:
				needProbe = true
			default:
				dev.State = device.StatePendingUp
				needProbe = true
			}
		})
		d.log.Debugf("lease hint mac=%s ip=%s host=%s", mac, e.IP, e.Hostname)
		if needProbe {
			d.scheduleProbe(ctx, mac)
		}
	}
}

func (d *Daemon) tickSuspect(ctx context.Context) {
	now := time.Now()
	for _, snap := range d.store.Snapshot() {
		if snap.State != device.StateSuspect && snap.State != device.StatePendingUp {
			continue
		}
		mac := snap.MAC
		var needProbe, becameOffline bool
		var ip, iface, name string
		d.store.Update(mac, func(dev *device.Device) {
			tr := device.ApplyEvent(dev, device.EventTimeout, d.params, now)
			needProbe = tr.NeedProbe
			becameOffline = tr.BecameOffline
			ip, iface, name = dev.IP, dev.Iface, dev.Name
		})
		if becameOffline {
			d.push(ctx, "下线", mac, ip, name, iface)
		}
		if needProbe {
			d.scheduleProbe(ctx, mac)
		}
	}
}

func (d *Daemon) scheduleProbe(ctx context.Context, mac string) {
	d.probeMu.Lock()
	next, ok := d.pending[mac]
	now := time.Now()
	if ok && now.Before(next) {
		d.probeMu.Unlock()
		return
	}
	// reserve slot with backoff from device
	var delay time.Duration
	d.store.Update(mac, func(dev *device.Device) {
		delay = device.NextBackoff(dev.ProbeIndex)
	})
	d.pending[mac] = now.Add(delay)
	d.probeMu.Unlock()

	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		d.runProbe(ctx, mac)
	}()
}

func (d *Daemon) clearProbeSchedule(mac string) {
	d.probeMu.Lock()
	delete(d.pending, mac)
	d.probeMu.Unlock()
}

func (d *Daemon) runProbe(ctx context.Context, mac string) {
	var ipStr, iface, name string
	var state device.State
	d.store.Update(mac, func(dev *device.Device) {
		ipStr, iface, name = dev.IP, dev.Iface, dev.Name
		state = dev.State
	})
	if state != device.StatePendingUp && state != device.StateSuspect {
		d.clearProbeSchedule(mac)
		return
	}
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() == nil {
		d.log.Debugf("probe skip mac=%s no ipv4", mac)
		d.clearProbeSchedule(mac)
		return
	}
	if iface == "" || skipIface(iface) {
		if resolved := resolveIface(ipStr); resolved != "" {
			iface = resolved
			d.store.Update(mac, func(dev *device.Device) { dev.Iface = iface })
		}
	}
	if iface == "" {
		d.log.Debugf("probe skip mac=%s ip=%s: no iface", mac, ipStr)
		d.clearProbeSchedule(mac)
		return
	}
	hw, _ := net.ParseMAC(mac)
	ok, err := d.pool.Do(ctx, iface, nil, ip.To4(), hw)
	if err != nil {
		d.log.Debugf("probe err mac=%s iface=%s: %v", mac, iface, err)
		ok = false
	}
	now := time.Now()
	var becameOnline, becameOffline, needProbe bool
	d.store.Update(mac, func(dev *device.Device) {
		var tr device.Transition
		if ok {
			tr = device.ApplyEvent(dev, device.EventProbeOK, d.params, now)
		} else {
			tr = device.ApplyEvent(dev, device.EventProbeFail, d.params, now)
		}
		becameOnline = tr.BecameOnline
		becameOffline = tr.BecameOffline
		needProbe = tr.NeedProbe
		ipStr, iface, name = dev.IP, dev.Iface, dev.Name
	})
	d.log.Debugf("probe mac=%s iface=%s ok=%v", mac, iface, ok)
	// Drop reservation before optional reschedule so backoff can be re-armed.
	d.clearProbeSchedule(mac)
	if becameOnline {
		d.push(ctx, "上线", mac, ipStr, name, iface)
	}
	if becameOffline {
		d.push(ctx, "下线", mac, ipStr, name, iface)
	}
	if needProbe {
		d.scheduleProbe(ctx, mac)
	}
}

func skipIface(iface string) bool {
	low := strings.ToLower(iface)
	return low == "lo" || low == "docker0" || strings.HasPrefix(low, "veth")
}

func (d *Daemon) ifaceAllowed(iface string) bool {
	if iface == "" {
		return true
	}
	if len(d.cfg.WatchIfaces) > 0 {
		for _, w := range d.cfg.WatchIfaces {
			if w == iface {
				return true
			}
		}
		return false
	}
	// Auto: if br-lan exists, only watch LAN bridges (ignore WAN/side nets on eth0).
	if _, err := net.InterfaceByName("br-lan"); err == nil {
		return iface == "br-lan" || iface == "br0" || strings.HasPrefix(iface, "lan")
	}
	return true
}

// resolveIface looks up the L2 device for an IPv4 from /proc/net/arp.
func resolveIface(ip string) string {
	b, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return ""
	}
	for i, line := range strings.Split(string(b), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		if fields[0] == ip {
			return fields[5]
		}
	}
	// Prefer common LAN bridges when ARP row missing.
	for _, name := range []string{"br-lan", "br0", "lan"} {
		if ifi, err := net.InterfaceByName(name); err == nil && ifi.Flags&net.FlagUp != 0 {
			return name
		}
	}
	return ""
}

func (d *Daemon) push(ctx context.Context, action, mac, ip, name, iface string) {
	now := time.Now()
	opts := notify.FormatDeviceOpts{}
	if action == "下线" {
		var since time.Time
		d.store.Update(mac, func(dev *device.Device) {
			since = dev.OnlineSince
		})
		if !since.IsZero() {
			opts.OnlineDuration = notify.FormatDuration(now.Sub(since))
		} else {
			opts.OnlineDuration = "0分"
		}
	}
	if d.cfg.NotifyListEnable {
		exclude := ""
		if action == "下线" {
			exclude = mac
		}
		opts.OnlineList = d.formatOnlineList(now, exclude)
	}
	msg := notify.FormatDevice(d.cfg.DeviceName, action, mac, ip, name, iface, opts)
	d.log.Infof("notify %s %s (%s)", action, name, mac)
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := d.sender.Send(cctx, msg); err != nil {
		d.log.Errorf("send: %v", err)
	}
}

func (d *Daemon) formatOnlineList(now time.Time, excludeMAC string) string {
	excludeMAC = config.NormalizeMAC(excludeMAC)
	items := make([]notify.OnlineListItem, 0, 32)
	for _, snap := range d.store.Snapshot() {
		if snap.State != device.StateOnline {
			continue
		}
		if excludeMAC != "" && config.NormalizeMAC(snap.MAC) == excludeMAC {
			continue
		}
		name := snap.Name
		if name == "" {
			name = "unknown"
		}
		items = append(items, notify.OnlineListItem{
			Name:        name,
			IP:          snap.IP,
			OnlineSince: snap.OnlineSince,
		})
	}
	return notify.FormatOnlineList(items, now, d.cfg.NotifyListMax)
}

func (d *Daemon) loadState() {
	path := d.cfg.StateFile
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var st struct {
		Devices []device.Device `json:"devices"`
	}
	if err := json.Unmarshal(b, &st); err != nil {
		d.log.Debugf("state load: %v", err)
		return
	}
	n := d.store.Restore(st.Devices)
	if n > 0 {
		d.log.Infof("restored %d devices from %s", n, path)
	}
}

func (d *Daemon) writeState() {
	type status struct {
		Running bool            `json:"running"`
		Version string          `json:"version"`
		Devices []device.Device `json:"devices"`
	}
	st := status{
		Running: true,
		Version: config.Version,
		Devices: d.store.Snapshot(),
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(d.cfg.StateFile), 0755)
	_ = os.WriteFile(d.cfg.StateFile, b, 0644)
}

func (d *Daemon) trimLog() {
	const maxBytes = 256 * 1024
	path := d.cfg.LogFile
	if path == "" || path == "/dev/null" {
		return
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() < maxBytes {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) < maxBytes/2 {
		return
	}
	// Keep last half to bound debug log growth.
	keep := b[len(b)-maxBytes/2:]
	if i := bytes.IndexByte(keep, '\n'); i >= 0 && i+1 < len(keep) {
		keep = keep[i+1:]
	}
	_ = os.WriteFile(path, keep, 0644)
}

// SendTest sends a test push using current config.
func SendTest(cfg config.Config) error {
	if err := notify.ValidateConfig(cfg); err != nil {
		return err
	}
	sender, err := notify.FromConfig(cfg)
	if err != nil {
		return err
	}
	msg := notify.Message{
		Title:   "[" + cfg.DeviceName + "] NetNotify 测试",
		Content: "这是一条来自 netnotifyd 的测试消息\n时间: " + time.Now().Format("2006-01-02 15:04:05"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return sender.Send(ctx, msg)
}
