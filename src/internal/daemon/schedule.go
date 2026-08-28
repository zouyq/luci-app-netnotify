package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/zouyq/netnotify/internal/config"
	"github.com/zouyq/netnotify/internal/device"
	"github.com/zouyq/netnotify/internal/netcheck"
	"github.com/zouyq/netnotify/internal/notify"
)

func (d *Daemon) tickCron(ctx context.Context) {
	mode := strings.TrimSpace(d.cfg.Crontab)
	if mode == "" || mode == "0" {
		return
	}
	now := time.Now()
	var key string
	switch mode {
	case "1":
		hour := now.Hour()
		match := false
		for _, h := range d.cfg.RegularTime {
			if h == hour {
				match = true
				break
			}
		}
		if !match {
			return
		}
		key = fmt.Sprintf("h-%s-%02d", now.Format("2006-01-02"), hour)
	case "2":
		d.cronMu.Lock()
		last := d.lastCronAt
		d.cronMu.Unlock()
		if !last.IsZero() && now.Sub(last) < time.Duration(d.cfg.IntervalHours)*time.Hour {
			return
		}
		key = fmt.Sprintf("i-%s-%d", now.Format("2006-01-02-15"), d.cfg.IntervalHours)
	default:
		return
	}

	d.cronMu.Lock()
	if d.lastCronKey == key {
		d.cronMu.Unlock()
		return
	}
	d.lastCronKey = key
	d.lastCronAt = now
	d.cronMu.Unlock()

	msg := d.buildCronMessage(now)
	d.log.Infof("cron push: %s", msg.Title)
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	if err := d.sender.Send(cctx, msg); err != nil {
		d.log.Errorf("cron send: %v", err)
		return
	}
	d.writeState()
}

func (d *Daemon) buildCronMessage(now time.Time) notify.Message {
	online := make([]notify.OnlineListItem, 0)
	total := 0
	for _, x := range d.store.Snapshot() {
		total++
		if x.State != device.StateOnline {
			continue
		}
		name := x.Name
		if name == "" {
			name = "unknown"
		}
		online = append(online, notify.OnlineListItem{
			Name:        name,
			IP:          x.IP,
			OnlineSince: x.OnlineSince,
		})
	}
	nc := d.snapshotNetcheck()
	return formatCronMessage(d.cfg, now, online, total, nc)
}

func formatCronMessage(cfg config.Config, now time.Time, online []notify.OnlineListItem, total int, nc NetcheckStatus) notify.Message {
	title := fmt.Sprintf("[%s] %s", cfg.DeviceName, cfg.SendTitle)
	var b strings.Builder
	b.WriteString(now.Format("2006-01-02 15:04:05"))
	b.WriteByte('\n')

	if cfg.CronStatus {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		b.WriteString(fmt.Sprintf("系统负载: %s\n", netcheck.LoadAvg()))
		b.WriteString(fmt.Sprintf("运行时长: %s\n", netcheck.FormatUptime(netcheck.UptimeSec())))
		b.WriteString(fmt.Sprintf("启动时间: %s\n", netcheck.BootTime().Format("2006-01-02 15:04:05")))
		wan := nc.WANIP
		if wan == "" {
			wan = netcheck.WANIPv4(cfg.NetcheckWANIface)
		}
		if wan == "" {
			wan = "-"
		}
		b.WriteString(fmt.Sprintf("WAN IP: %s\n", wan))
		b.WriteString(fmt.Sprintf("内存约: %dMB | 协程: %d\n", ms.Sys/1024/1024, runtime.NumGoroutine()))
		if cfg.NetcheckEnable || nc.Enabled {
			ok := "未知"
			if !nc.LastCheck.IsZero() || !nc.LastOK.IsZero() {
				if nc.OK {
					ok = "正常"
				} else {
					ok = "异常"
				}
			}
			b.WriteString(fmt.Sprintf("外网检测: %s", ok))
			if nc.LastDetail != "" {
				b.WriteString(" (" + nc.LastDetail + ")")
			}
			b.WriteByte('\n')
		}
	}

	if cfg.CronDevices {
		b.WriteByte('\n')
		if len(online) == 0 {
			b.WriteString(fmt.Sprintf("当前在线 (0/%d): 无\n", total))
		} else {
			max := cfg.NotifyListMax
			if max <= 0 {
				max = 15
			}
			b.WriteString(notify.FormatOnlineList(online, now, max))
			b.WriteByte('\n')
		}
	}

	return notify.Message{Title: title, Content: strings.TrimSpace(b.String())}
}

// SendCronNow forces one scheduled-style report using live device snapshot when possible.
func SendCronNow(cfg config.Config) error {
	if err := notify.ValidateConfig(cfg); err != nil {
		return err
	}
	sender, err := notify.FromConfig(cfg)
	if err != nil {
		return err
	}
	now := time.Now()
	online := make([]notify.OnlineListItem, 0)
	total := 0
	nc := NetcheckStatus{Enabled: cfg.NetcheckEnable}
	if b, err := os.ReadFile(cfg.StateFile); err == nil {
		var st struct {
			Devices  []device.Device `json:"devices"`
			Netcheck NetcheckStatus  `json:"netcheck"`
		}
		if json.Unmarshal(b, &st) == nil {
			nc = st.Netcheck
			for _, x := range st.Devices {
				total++
				if x.State != device.StateOnline {
					continue
				}
				name := x.Name
				if name == "" {
					name = "unknown"
				}
				online = append(online, notify.OnlineListItem{
					Name:        name,
					IP:          x.IP,
					OnlineSince: x.OnlineSince,
				})
			}
		}
	}
	msg := formatCronMessage(cfg, now, online, total, nc)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	return sender.Send(ctx, msg)
}
