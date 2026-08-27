package daemon

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/zouyq/netnotify/internal/config"
	"github.com/zouyq/netnotify/internal/device"
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
	}
}

func (d *Daemon) buildCronMessage(now time.Time) notify.Message {
	title := fmt.Sprintf("[%s] %s", d.cfg.DeviceName, d.cfg.SendTitle)
	var b strings.Builder
	b.WriteString(now.Format("2006-01-02 15:04:05"))
	b.WriteByte('\n')

	if d.cfg.CronStatus {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		load := readLoadAvg()
		b.WriteString(fmt.Sprintf("系统: load %s | RSS≈%dMB | goroutines=%d\n",
			load, ms.Sys/1024/1024, runtime.NumGoroutine()))
	}

	if d.cfg.CronDevices {
		devs := d.store.Snapshot()
		online := 0
		for _, x := range devs {
			if x.State == device.StateOnline {
				online++
			}
		}
		b.WriteString(fmt.Sprintf("在线设备: %d / %d\n", online, len(devs)))
		for _, x := range devs {
			if x.State != device.StateOnline {
				continue
			}
			b.WriteString(fmt.Sprintf("- %s %s (%s)\n", x.Name, x.IP, x.MAC))
		}
	}

	return notify.Message{Title: title, Content: strings.TrimSpace(b.String())}
}

func readLoadAvg() string {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "?"
	}
	fields := strings.Fields(string(b))
	if len(fields) >= 3 {
		return fields[0] + " " + fields[1] + " " + fields[2]
	}
	return strings.TrimSpace(string(b))
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
	// Prefer reading current state file written by running daemon.
	content := cfg.SendTitle + "\n" + time.Now().Format("2006-01-02 15:04:05")
	if b, err := os.ReadFile(cfg.StateFile); err == nil {
		content = content + "\n(see devices.json on router; snapshot attached below)\n" + string(b)
		if len(content) > 3500 {
			content = content[:3500] + "\n..."
		}
	}
	msg := notify.Message{
		Title:   fmt.Sprintf("[%s] %s", cfg.DeviceName, cfg.SendTitle),
		Content: content,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	return sender.Send(ctx, msg)
}
