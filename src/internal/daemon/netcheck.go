package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zouyq/netnotify/internal/device"
	"github.com/zouyq/netnotify/internal/netcheck"
	"github.com/zouyq/netnotify/internal/notify"
)

// NetcheckStatus is exposed in devices.json for LuCI.
type NetcheckStatus struct {
	Enabled    bool      `json:"enabled"`
	OK         bool      `json:"ok"`
	LastCheck  time.Time `json:"last_check,omitempty"`
	LastOK     time.Time `json:"last_ok,omitempty"`
	LastFail   time.Time `json:"last_fail,omitempty"`
	LastAction string    `json:"last_action,omitempty"`
	LastDetail string    `json:"last_detail,omitempty"`
	WANIP      string    `json:"wan_ip,omitempty"`
	LoadAvg    string    `json:"loadavg,omitempty"`
	Uptime     string    `json:"uptime,omitempty"`
}

func (d *Daemon) netcheckParams() netcheck.Params {
	return netcheck.Params{
		Hosts:            d.cfg.NetcheckHosts,
		IPs:              d.cfg.NetcheckIPs,
		Retry:            d.cfg.NetcheckRetry,
		RetryIntervalSec: d.cfg.NetcheckRetryIntervalSec,
		TimeoutSec:       d.cfg.NetcheckTimeoutSec,
		WANIface:         d.cfg.NetcheckWANIface,
		CooldownSec:      d.cfg.NetcheckCooldownSec,
	}
}

func (d *Daemon) runNetcheckLoop(ctx context.Context) {
	if !d.cfg.NetcheckEnable {
		return
	}
	d.log.Infof("netcheck enabled (interval=%ds wan=%s)", d.cfg.NetcheckIntervalSec, d.cfg.NetcheckWANIface)
	d.setNetcheckStatus(func(st *NetcheckStatus) {
		st.Enabled = true
		st.LastAction = "starting"
	})

	p := d.netcheckParams()
	wait := time.Duration(d.cfg.NetcheckStartupWaitSec) * time.Second
	if wait > 0 {
		d.log.Infof("netcheck waiting up to %s for network...", wait)
		_ = d.netChecker.WaitReachable(ctx, p, wait)
	}

	// Run first check soon, then on interval.
	ticker := time.NewTicker(time.Duration(d.cfg.NetcheckIntervalSec) * time.Second)
	defer ticker.Stop()
	d.doNetcheck(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.doNetcheck(ctx)
		}
	}
}

func (d *Daemon) doNetcheck(ctx context.Context) {
	if !d.cfg.NetcheckEnable {
		return
	}
	p := d.netcheckParams()
	now := time.Now()

	ok, via, detail := d.netChecker.CheckHosts(ctx, p)
	if ok {
		d.log.Debugf("netcheck ok via host %s", via)
		d.setNetcheckStatus(func(st *NetcheckStatus) {
			st.Enabled = true
			st.OK = true
			st.LastCheck = now
			st.LastOK = now
			st.LastAction = "ok"
			st.LastDetail = "host:" + via + " " + detail
			st.WANIP = netcheck.WANIPv4(d.cfg.NetcheckWANIface)
			st.LoadAvg = netcheck.LoadAvg()
			st.Uptime = netcheck.FormatUptime(netcheck.UptimeSec())
		})
		return
	}
	d.log.Infof("netcheck hosts failed, trying ips...")
	ok, via, detail = d.netChecker.CheckIPs(ctx, p)
	if ok {
		d.log.Debugf("netcheck ok via ip %s", via)
		d.setNetcheckStatus(func(st *NetcheckStatus) {
			st.Enabled = true
			st.OK = true
			st.LastCheck = now
			st.LastOK = now
			st.LastAction = "ok"
			st.LastDetail = "ip:" + via + " " + detail
			st.WANIP = netcheck.WANIPv4(d.cfg.NetcheckWANIface)
			st.LoadAvg = netcheck.LoadAvg()
			st.Uptime = netcheck.FormatUptime(netcheck.UptimeSec())
		})
		return
	}

	d.log.Infof("netcheck unreachable: %s", detail)
	d.setNetcheckStatus(func(st *NetcheckStatus) {
		st.Enabled = true
		st.OK = false
		st.LastCheck = now
		st.LastFail = now
		st.LastAction = "fail"
		st.LastDetail = detail
	})

	d.netcheckMu.Lock()
	lastRestart := d.netcheckLastRestart
	d.netcheckMu.Unlock()
	cool := time.Duration(d.cfg.NetcheckCooldownSec) * time.Second
	if !lastRestart.IsZero() && time.Since(lastRestart) < cool {
		d.log.Infof("netcheck wan restart skipped (cooldown %s left)", cool-time.Since(lastRestart))
		d.setNetcheckStatus(func(st *NetcheckStatus) {
			st.LastAction = "cooldown"
			st.LastDetail = "wan restart skipped due to cooldown"
		})
		return
	}

	d.log.Infof("netcheck restarting wan iface=%s", d.cfg.NetcheckWANIface)
	if err := netcheck.RestartWAN(d.cfg.NetcheckWANIface); err != nil {
		d.log.Errorf("netcheck ifup: %v", err)
		d.setNetcheckStatus(func(st *NetcheckStatus) {
			st.LastAction = "ifup_error"
			st.LastDetail = err.Error()
		})
		return
	}
	d.netcheckMu.Lock()
	d.netcheckLastRestart = time.Now()
	d.netcheckMu.Unlock()
	d.setNetcheckStatus(func(st *NetcheckStatus) {
		st.LastAction = "wan_restart"
		st.LastDetail = "ifup " + d.cfg.NetcheckWANIface
	})

	recoverWait := time.Duration(d.cfg.NetcheckRecoverWaitSec) * time.Second
	time.Sleep(5 * time.Second)
	recovered := d.netChecker.WaitReachable(ctx, p, recoverWait)
	wanIP := netcheck.WANIPv4(d.cfg.NetcheckWANIface)
	load := netcheck.LoadAvg()
	uptime := netcheck.FormatUptime(netcheck.UptimeSec())
	if recovered {
		d.log.Infof("netcheck recovered wan_ip=%s", wanIP)
		d.setNetcheckStatus(func(st *NetcheckStatus) {
			st.OK = true
			st.LastOK = time.Now()
			st.LastAction = "recovered"
			st.LastDetail = "network restored after wan restart"
			st.WANIP = wanIP
			st.LoadAvg = load
			st.Uptime = uptime
		})
		if d.cfg.NetcheckPushOnRecover {
			d.pushNetcheckRecover(ctx, wanIP, load, uptime)
		}
	} else {
		d.log.Infof("netcheck still down after wan restart")
		d.setNetcheckStatus(func(st *NetcheckStatus) {
			st.OK = false
			st.LastFail = time.Now()
			st.LastAction = "recover_timeout"
			st.LastDetail = "still unreachable after wan restart"
			st.WANIP = wanIP
			st.LoadAvg = load
			st.Uptime = uptime
		})
	}
}

func (d *Daemon) setNetcheckStatus(fn func(*NetcheckStatus)) {
	d.netcheckMu.Lock()
	defer d.netcheckMu.Unlock()
	fn(&d.netcheckStatus)
}

func (d *Daemon) snapshotNetcheck() NetcheckStatus {
	d.netcheckMu.Lock()
	defer d.netcheckMu.Unlock()
	st := d.netcheckStatus
	st.Enabled = d.cfg.NetcheckEnable
	if st.WANIP == "" {
		st.WANIP = netcheck.WANIPv4(d.cfg.NetcheckWANIface)
	}
	st.LoadAvg = netcheck.LoadAvg()
	st.Uptime = netcheck.FormatUptime(netcheck.UptimeSec())
	return st
}

func (d *Daemon) pushNetcheckRecover(ctx context.Context, wanIP, load, uptime string) {
	online := 0
	for _, snap := range d.store.Snapshot() {
		if snap.State == device.StateOnline {
			online++
		}
	}
	if wanIP == "" {
		wanIP = "-"
	}
	title := fmt.Sprintf("[%s] 网络已恢复", d.cfg.DeviceName)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("WAN IP: %s\n", wanIP))
	b.WriteString(fmt.Sprintf("在线设备: %d\n", online))
	b.WriteString(fmt.Sprintf("系统负载: %s\n", load))
	b.WriteString(fmt.Sprintf("启动时间: %s\n", netcheck.BootTime().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("运行时长: %s\n", uptime))
	b.WriteString(fmt.Sprintf("时间: %s", time.Now().Format("2006-01-02 15:04:05")))
	msg := notify.Message{Title: title, Content: b.String()}
	d.log.Infof("notify netcheck recovered")
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := d.sender.Send(cctx, msg); err != nil {
		d.log.Errorf("netcheck push: %v", err)
	}
}
