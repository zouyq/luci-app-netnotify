package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const Version = "0.3.0"

// Config holds runtime settings for netnotifyd.
type Config struct {
	Enable            bool              `json:"enable"`
	DeviceName        string            `json:"device_name"`
	Channel           string            `json:"channel"`
	WebhookURL        string            `json:"webhook_url"`
	WebhookTemplate   string            `json:"webhook_template"`
	SuspectTimeoutSec int               `json:"suspect_timeout_sec"`
	ProbeMaxParallel  int               `json:"probe_max_parallel"`
	OfflineFailCount  int               `json:"offline_fail_count"`
	LeasePollSec      int               `json:"lease_poll_sec"`
	Debug             bool              `json:"debug"`
	Aliases           map[string]string `json:"aliases"`
	Whitelist         []string          `json:"whitelist"`
	Blacklist         []string          `json:"blacklist"`
	WatchIfaces       []string          `json:"watch_ifaces"`
	DHCPLeasesPath    string            `json:"dhcp_leases_path"`
	StateFile         string            `json:"state_file"`
	LogFile           string            `json:"log_file"`
	OUIEnable         bool              `json:"oui_enable"`
	OUIPath           string            `json:"oui_path"`

	// Bark
	BarkToken  string `json:"bark_token"`
	BarkServer string `json:"bark_srv"`
	BarkSound  string `json:"bark_sound"`
	BarkIcon   string `json:"bark_icon"`
	BarkLevel  string `json:"bark_level"`

	// WeCom application
	QywxCorpID     string `json:"qywx_corpid"`
	QywxAgentID    string `json:"qywx_agentid"`
	QywxCorpSecret string `json:"qywx_corpsecret"`
	QywxToUser     string `json:"qywx_touser"`

	// Scheduled report: "" off, "1" fixed hours, "2" interval hours
	Crontab       string `json:"crontab"`
	RegularTime   []int  `json:"regular_time"` // hours 0-23
	IntervalHours int    `json:"interval_time"`
	SendTitle     string `json:"send_title"`
	CronDevices   bool   `json:"cron_devices"`
	CronStatus    bool   `json:"cron_status"`

	// Online list appended to up/down pushes
	NotifyListEnable bool `json:"notify_list_enable"`
	NotifyListMax    int  `json:"notify_list_max"`

	// WAN connectivity watchdog (from network_check.sh)
	NetcheckEnable           bool     `json:"netcheck_enable"`
	NetcheckHosts            []string `json:"netcheck_hosts"`
	NetcheckIPs              []string `json:"netcheck_ips"`
	NetcheckIntervalSec      int      `json:"netcheck_interval_sec"`
	NetcheckTimeoutSec       int      `json:"netcheck_timeout_sec"`
	NetcheckRetry            int      `json:"netcheck_retry"`
	NetcheckRetryIntervalSec int      `json:"netcheck_retry_interval_sec"`
	NetcheckWANIface         string   `json:"netcheck_wan_iface"`
	NetcheckStartupWaitSec   int      `json:"netcheck_startup_wait_sec"`
	NetcheckCooldownSec      int      `json:"netcheck_cooldown_sec"`
	NetcheckRecoverWaitSec   int      `json:"netcheck_recover_wait_sec"`
	NetcheckPushOnRecover    bool     `json:"netcheck_push_on_recover"`
}

// Defaults returns safe defaults (disabled until configured).
func Defaults() Config {
	return Config{
		Enable:            false,
		DeviceName:        "OpenWrt",
		Channel:           "webhook",
		WebhookURL:        "",
		SuspectTimeoutSec: 60,
		ProbeMaxParallel:  2,
		OfflineFailCount:  3,
		LeasePollSec:      30,
		Debug:             false,
		Aliases:           map[string]string{},
		DHCPLeasesPath:    "/var/dhcp.leases",
		StateFile:         "/tmp/netnotify/devices.json",
		LogFile:           "/tmp/netnotify/netnotify.log",
		OUIEnable:         true,
		OUIPath:           "/usr/share/netnotify/oui_base.txt",
		BarkSound:         "silence.caf",
		BarkLevel:         "active",
		QywxToUser:        "@all",
		Crontab:           "",
		IntervalHours:     6,
		SendTitle:         "路由状态",
		CronDevices:       true,
		CronStatus:        true,
		NotifyListEnable:  true,
		NotifyListMax:     15,
		NetcheckEnable:    false,
		NetcheckHosts: []string{
			"connect.rom.miui.com",
			"connectivitycheck.vivo.com.cn",
			"connectivitycheck.platform.hicloud.com",
			"conn1.oppomobile.com",
		},
		NetcheckIPs:              []string{"223.5.5.5", "119.29.29.29"},
		NetcheckIntervalSec:      300,
		NetcheckTimeoutSec:       2,
		NetcheckRetry:            2,
		NetcheckRetryIntervalSec: 1,
		NetcheckWANIface:         "wan",
		NetcheckStartupWaitSec:   120,
		NetcheckCooldownSec:      600,
		NetcheckRecoverWaitSec:   120,
		NetcheckPushOnRecover:    true,
	}
}

// Load loads config from -config JSON path, else UCI/netnotify, else defaults.
func Load(path string) (Config, error) {
	cfg := Defaults()
	if path != "" {
		if err := loadJSON(path, &cfg); err != nil {
			return cfg, err
		}
		normalize(&cfg)
		return cfg, nil
	}
	if _, err := os.Stat("/etc/netnotify.json"); err == nil {
		if err := loadJSON("/etc/netnotify.json", &cfg); err != nil {
			return cfg, err
		}
		normalize(&cfg)
		return cfg, nil
	}
	if err := loadUCI(&cfg); err != nil {
		_ = err
	}
	normalize(&cfg)
	return cfg, nil
}

func normalize(cfg *Config) {
	if cfg.SuspectTimeoutSec <= 0 {
		cfg.SuspectTimeoutSec = 60
	}
	if cfg.ProbeMaxParallel <= 0 {
		cfg.ProbeMaxParallel = 2
	}
	if cfg.ProbeMaxParallel > 2 {
		cfg.ProbeMaxParallel = 2
	}
	if cfg.OfflineFailCount <= 0 {
		cfg.OfflineFailCount = 3
	}
	if cfg.LeasePollSec <= 0 {
		cfg.LeasePollSec = 30
	}
	if cfg.DeviceName == "" {
		cfg.DeviceName = "OpenWrt"
	}
	if cfg.Aliases == nil {
		cfg.Aliases = map[string]string{}
	}
	if cfg.DHCPLeasesPath == "" {
		cfg.DHCPLeasesPath = "/var/dhcp.leases"
	}
	if cfg.StateFile == "" {
		cfg.StateFile = "/tmp/netnotify/devices.json"
	}
	if cfg.LogFile == "" {
		cfg.LogFile = "/tmp/netnotify/netnotify.log"
	}
	if cfg.OUIPath == "" {
		cfg.OUIPath = "/usr/share/netnotify/oui_base.txt"
	}
	if cfg.BarkLevel == "" {
		cfg.BarkLevel = "active"
	}
	if cfg.QywxToUser == "" {
		cfg.QywxToUser = "@all"
	}
	if cfg.IntervalHours <= 0 {
		cfg.IntervalHours = 6
	}
	if cfg.SendTitle == "" {
		cfg.SendTitle = "路由状态"
	}
	if cfg.NotifyListMax <= 0 {
		cfg.NotifyListMax = 15
	}
	if cfg.NotifyListMax > 50 {
		cfg.NotifyListMax = 50
	}
	if len(cfg.NetcheckHosts) == 0 {
		cfg.NetcheckHosts = []string{
			"connect.rom.miui.com",
			"connectivitycheck.vivo.com.cn",
			"connectivitycheck.platform.hicloud.com",
			"conn1.oppomobile.com",
		}
	}
	if len(cfg.NetcheckIPs) == 0 {
		cfg.NetcheckIPs = []string{"223.5.5.5", "119.29.29.29"}
	}
	if cfg.NetcheckIntervalSec <= 0 {
		cfg.NetcheckIntervalSec = 300
	}
	if cfg.NetcheckTimeoutSec <= 0 {
		cfg.NetcheckTimeoutSec = 2
	}
	if cfg.NetcheckRetry <= 0 {
		cfg.NetcheckRetry = 2
	}
	if cfg.NetcheckRetryIntervalSec <= 0 {
		cfg.NetcheckRetryIntervalSec = 1
	}
	if cfg.NetcheckWANIface == "" {
		cfg.NetcheckWANIface = "wan"
	}
	if cfg.NetcheckStartupWaitSec < 0 {
		cfg.NetcheckStartupWaitSec = 0
	}
	if cfg.NetcheckCooldownSec <= 0 {
		cfg.NetcheckCooldownSec = 600
	}
	if cfg.NetcheckRecoverWaitSec <= 0 {
		cfg.NetcheckRecoverWaitSec = 120
	}
	norm := make(map[string]string, len(cfg.Aliases))
	for k, v := range cfg.Aliases {
		norm[NormalizeMAC(k)] = v
	}
	cfg.Aliases = norm
	for i := range cfg.Whitelist {
		cfg.Whitelist[i] = NormalizeMAC(cfg.Whitelist[i])
	}
	for i := range cfg.Blacklist {
		cfg.Blacklist[i] = NormalizeMAC(cfg.Blacklist[i])
	}
}

func loadJSON(path string, cfg *Config) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, cfg)
}

func loadUCI(cfg *Config) error {
	get := func(opt string) string {
		out, err := exec.Command("uci", "-q", "get", "netnotify.main."+opt).CombinedOutput()
		if err != nil {
			out, err = exec.Command("uci", "-q", "get", "netnotify.@netnotify[0]."+opt).CombinedOutput()
			if err != nil {
				return ""
			}
		}
		return strings.TrimSpace(string(out))
	}
	list := func(opt string) []string {
		out, err := exec.Command("uci", "-q", "get", "netnotify.main."+opt).CombinedOutput()
		if err != nil {
			out, err = exec.Command("uci", "-q", "get", "netnotify.@netnotify[0]."+opt).CombinedOutput()
			if err != nil {
				return nil
			}
		}
		s := strings.TrimSpace(string(out))
		if s == "" {
			return nil
		}
		return strings.Fields(s)
	}
	atoi := func(opt string, dst *int) {
		if v := get(opt); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				*dst = n
			}
		}
	}
	flag := func(opt string, dst *bool) {
		if v := get(opt); v != "" {
			*dst = v == "1" || strings.EqualFold(v, "true")
		}
	}
	str := func(opt string, dst *string) {
		if v := get(opt); v != "" {
			*dst = v
		}
	}

	flag("enable", &cfg.Enable)
	str("device_name", &cfg.DeviceName)
	str("channel", &cfg.Channel)
	str("webhook_url", &cfg.WebhookURL)
	str("webhook_template", &cfg.WebhookTemplate)
	atoi("suspect_timeout_sec", &cfg.SuspectTimeoutSec)
	atoi("probe_max_parallel", &cfg.ProbeMaxParallel)
	atoi("offline_fail_count", &cfg.OfflineFailCount)
	if v := get("sleeptime"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.LeasePollSec = n
		}
	}
	flag("debug", &cfg.Debug)
	flag("oui_enable", &cfg.OUIEnable)
	str("oui_path", &cfg.OUIPath)

	str("bark_token", &cfg.BarkToken)
	str("bark_srv", &cfg.BarkServer)
	str("bark_sound", &cfg.BarkSound)
	str("bark_icon", &cfg.BarkIcon)
	str("bark_level", &cfg.BarkLevel)

	str("qywx_corpid", &cfg.QywxCorpID)
	str("qywx_agentid", &cfg.QywxAgentID)
	str("qywx_corpsecret", &cfg.QywxCorpSecret)
	str("qywx_touser", &cfg.QywxToUser)

	str("crontab", &cfg.Crontab)
	str("send_title", &cfg.SendTitle)
	atoi("interval_time", &cfg.IntervalHours)
	flag("cron_devices", &cfg.CronDevices)
	flag("cron_status", &cfg.CronStatus)
	flag("notify_list_enable", &cfg.NotifyListEnable)
	atoi("notify_list_max", &cfg.NotifyListMax)

	flag("netcheck_enable", &cfg.NetcheckEnable)
	atoi("netcheck_interval_sec", &cfg.NetcheckIntervalSec)
	atoi("netcheck_timeout_sec", &cfg.NetcheckTimeoutSec)
	atoi("netcheck_retry", &cfg.NetcheckRetry)
	atoi("netcheck_retry_interval_sec", &cfg.NetcheckRetryIntervalSec)
	str("netcheck_wan_iface", &cfg.NetcheckWANIface)
	atoi("netcheck_startup_wait_sec", &cfg.NetcheckStartupWaitSec)
	atoi("netcheck_cooldown_sec", &cfg.NetcheckCooldownSec)
	atoi("netcheck_recover_wait_sec", &cfg.NetcheckRecoverWaitSec)
	flag("netcheck_push_on_recover", &cfg.NetcheckPushOnRecover)
	if hosts := list("netcheck_hosts"); len(hosts) > 0 {
		cfg.NetcheckHosts = hosts
	}
	if ips := list("netcheck_ips"); len(ips) > 0 {
		cfg.NetcheckIPs = ips
	}

	cfg.RegularTime = nil
	for _, key := range []string{"regular_time", "regular_time_2", "regular_time_3"} {
		if v := get(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
				cfg.RegularTime = append(cfg.RegularTime, n)
			}
		}
	}

	cfg.Whitelist = list("whitelist")
	cfg.Blacklist = list("blacklist")
	cfg.WatchIfaces = list("watch_ifaces")
	for _, a := range list("aliases") {
		parts := strings.SplitN(a, "=", 2)
		if len(parts) == 2 {
			cfg.Aliases[NormalizeMAC(parts[0])] = parts[1]
		}
	}

	if _, err := exec.LookPath("uci"); err != nil {
		return parseUCIFile("/etc/config/netnotify", cfg)
	}
	return nil
}

func parseUCIFile(path string, cfg *Config) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	hostsReset, ipsReset := false, false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "option ") && !strings.HasPrefix(line, "list ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		key := fields[1]
		val := strings.Trim(strings.Join(fields[2:], " "), "'\"")
		switch {
		case fields[0] == "option" && key == "enable":
			cfg.Enable = val == "1"
		case fields[0] == "option" && key == "device_name":
			cfg.DeviceName = val
		case fields[0] == "option" && key == "channel":
			cfg.Channel = val
		case fields[0] == "option" && key == "webhook_url":
			cfg.WebhookURL = val
		case fields[0] == "option" && key == "bark_token":
			cfg.BarkToken = val
		case fields[0] == "option" && key == "bark_srv":
			cfg.BarkServer = val
		case fields[0] == "option" && key == "bark_sound":
			cfg.BarkSound = val
		case fields[0] == "option" && key == "bark_icon":
			cfg.BarkIcon = val
		case fields[0] == "option" && key == "bark_level":
			cfg.BarkLevel = val
		case fields[0] == "option" && key == "qywx_corpid":
			cfg.QywxCorpID = val
		case fields[0] == "option" && key == "qywx_agentid":
			cfg.QywxAgentID = val
		case fields[0] == "option" && key == "qywx_corpsecret":
			cfg.QywxCorpSecret = val
		case fields[0] == "option" && key == "qywx_touser":
			cfg.QywxToUser = val
		case fields[0] == "option" && key == "crontab":
			cfg.Crontab = val
		case fields[0] == "option" && key == "send_title":
			cfg.SendTitle = val
		case fields[0] == "option" && key == "interval_time":
			if n, e := strconv.Atoi(val); e == nil {
				cfg.IntervalHours = n
			}
		case fields[0] == "option" && key == "cron_devices":
			cfg.CronDevices = val == "1"
		case fields[0] == "option" && key == "cron_status":
			cfg.CronStatus = val == "1"
		case fields[0] == "option" && key == "notify_list_enable":
			cfg.NotifyListEnable = val == "1"
		case fields[0] == "option" && key == "notify_list_max":
			if n, e := strconv.Atoi(val); e == nil {
				cfg.NotifyListMax = n
			}
		case fields[0] == "option" && key == "netcheck_enable":
			cfg.NetcheckEnable = val == "1"
		case fields[0] == "option" && key == "netcheck_interval_sec":
			if n, e := strconv.Atoi(val); e == nil {
				cfg.NetcheckIntervalSec = n
			}
		case fields[0] == "option" && key == "netcheck_timeout_sec":
			if n, e := strconv.Atoi(val); e == nil {
				cfg.NetcheckTimeoutSec = n
			}
		case fields[0] == "option" && key == "netcheck_retry":
			if n, e := strconv.Atoi(val); e == nil {
				cfg.NetcheckRetry = n
			}
		case fields[0] == "option" && key == "netcheck_retry_interval_sec":
			if n, e := strconv.Atoi(val); e == nil {
				cfg.NetcheckRetryIntervalSec = n
			}
		case fields[0] == "option" && key == "netcheck_wan_iface":
			cfg.NetcheckWANIface = val
		case fields[0] == "option" && key == "netcheck_startup_wait_sec":
			if n, e := strconv.Atoi(val); e == nil {
				cfg.NetcheckStartupWaitSec = n
			}
		case fields[0] == "option" && key == "netcheck_cooldown_sec":
			if n, e := strconv.Atoi(val); e == nil {
				cfg.NetcheckCooldownSec = n
			}
		case fields[0] == "option" && key == "netcheck_recover_wait_sec":
			if n, e := strconv.Atoi(val); e == nil {
				cfg.NetcheckRecoverWaitSec = n
			}
		case fields[0] == "option" && key == "netcheck_push_on_recover":
			cfg.NetcheckPushOnRecover = val == "1"
		case fields[0] == "list" && key == "netcheck_hosts":
			if !hostsReset {
				cfg.NetcheckHosts = nil
				hostsReset = true
			}
			cfg.NetcheckHosts = append(cfg.NetcheckHosts, val)
		case fields[0] == "list" && key == "netcheck_ips":
			if !ipsReset {
				cfg.NetcheckIPs = nil
				ipsReset = true
			}
			cfg.NetcheckIPs = append(cfg.NetcheckIPs, val)
		case fields[0] == "option" && (key == "regular_time" || key == "regular_time_2" || key == "regular_time_3"):
			if n, e := strconv.Atoi(val); e == nil && n >= 0 && n <= 23 {
				cfg.RegularTime = append(cfg.RegularTime, n)
			}
		case fields[0] == "option" && key == "debug":
			cfg.Debug = val == "1"
		case fields[0] == "list" && key == "aliases":
			parts := strings.SplitN(val, "=", 2)
			if len(parts) == 2 {
				cfg.Aliases[NormalizeMAC(parts[0])] = parts[1]
			}
		case fields[0] == "list" && key == "whitelist":
			cfg.Whitelist = append(cfg.Whitelist, NormalizeMAC(val))
		case fields[0] == "list" && key == "blacklist":
			cfg.Blacklist = append(cfg.Blacklist, NormalizeMAC(val))
		case fields[0] == "list" && key == "watch_ifaces":
			cfg.WatchIfaces = append(cfg.WatchIfaces, val)
		}
	}
	return nil
}

// NormalizeMAC returns lower-case colon-separated MAC.
func NormalizeMAC(mac string) string {
	mac = strings.TrimSpace(strings.ToLower(mac))
	mac = strings.ReplaceAll(mac, "-", ":")
	mac = strings.ReplaceAll(mac, ".", ":")
	parts := strings.Split(mac, ":")
	if len(parts) == 6 {
		for i, p := range parts {
			if len(p) == 1 {
				parts[i] = "0" + p
			}
		}
		return strings.Join(parts, ":")
	}
	hex := strings.ReplaceAll(mac, ":", "")
	if len(hex) == 12 {
		var b strings.Builder
		for i := 0; i < 12; i += 2 {
			if i > 0 {
				b.WriteByte(':')
			}
			b.WriteString(hex[i : i+2])
		}
		return b.String()
	}
	return mac
}

// Allowed reports whether MAC passes white/black lists.
func (c Config) Allowed(mac string) bool {
	mac = NormalizeMAC(mac)
	for _, b := range c.Blacklist {
		if b == mac {
			return false
		}
	}
	if len(c.Whitelist) == 0 {
		return true
	}
	for _, w := range c.Whitelist {
		if w == mac {
			return true
		}
	}
	return false
}

// ResolveName picks alias, then dhcp hostname, then OUI vendor, else unknown.
// ouiLookup may be nil.
func (c Config) ResolveName(mac, dhcpHost string, ouiLookup func(string) string) string {
	mac = NormalizeMAC(mac)
	if n, ok := c.Aliases[mac]; ok && n != "" {
		return n
	}
	if dhcpHost != "" && dhcpHost != "*" && !strings.EqualFold(dhcpHost, "unknown") {
		return dhcpHost
	}
	if ouiLookup != nil {
		if v := ouiLookup(mac); v != "" {
			return v
		}
	}
	return "unknown"
}

func (c Config) String() string {
	return fmt.Sprintf("enable=%v channel=%s suspect=%ds parallel=%d offline_fails=%d cron=%s debug=%v",
		c.Enable, c.Channel, c.SuspectTimeoutSec, c.ProbeMaxParallel, c.OfflineFailCount, c.Crontab, c.Debug)
}
