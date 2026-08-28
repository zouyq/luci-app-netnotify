package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zouyq/netnotify/internal/config"
)

// Message is a push notification payload.
type Message struct {
	Title   string
	Content string
}

// Sender sends notifications.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

var sharedHTTP = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        4,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     60 * time.Second,
		DisableCompression:  true,
	},
}

// FromConfig builds a channel sender from full config.
func FromConfig(cfg config.Config) (Sender, error) {
	ch := strings.ToLower(strings.TrimSpace(cfg.Channel))
	switch ch {
	case "dingtalk", "dingding":
		if cfg.WebhookURL == "" {
			return &Noop{}, nil
		}
		return &DingTalk{URL: cfg.WebhookURL}, nil
	case "wecom_bot", "wecom", "qywx_bot":
		if cfg.WebhookURL == "" {
			return &Noop{}, nil
		}
		return &WeComBot{URL: cfg.WebhookURL}, nil
	case "wecom_app", "qywx", "qywx_app":
		if cfg.QywxCorpID == "" || cfg.QywxCorpSecret == "" || cfg.QywxAgentID == "" {
			return &Noop{}, nil
		}
		return &WeComApp{
			CorpID:     cfg.QywxCorpID,
			CorpSecret: cfg.QywxCorpSecret,
			AgentID:    cfg.QywxAgentID,
			ToUser:     cfg.QywxToUser,
		}, nil
	case "bark":
		if cfg.BarkToken == "" {
			return &Noop{}, nil
		}
		srv := strings.TrimRight(cfg.BarkServer, "/")
		if srv == "" {
			srv = "https://api.day.app"
		}
		return &Bark{
			Server: srv,
			Token:  cfg.BarkToken,
			Sound:  cfg.BarkSound,
			Icon:   cfg.BarkIcon,
			Level:  cfg.BarkLevel,
			Group:  cfg.DeviceName,
		}, nil
	case "webhook", "custom", "":
		if cfg.WebhookURL == "" {
			return &Noop{}, nil
		}
		return &Webhook{URL: cfg.WebhookURL, Template: cfg.WebhookTemplate}, nil
	default:
		if cfg.WebhookURL == "" {
			return &Noop{}, nil
		}
		return &Webhook{URL: cfg.WebhookURL}, nil
	}
}

// New is kept for tests; prefer FromConfig.
func New(channel, webhookURL string) (Sender, error) {
	return FromConfig(config.Config{Channel: channel, WebhookURL: webhookURL})
}

// ValidateConfig reports whether the selected channel has required fields.
func ValidateConfig(cfg config.Config) error {
	switch strings.ToLower(cfg.Channel) {
	case "dingtalk", "dingding", "wecom_bot", "wecom", "qywx_bot", "webhook", "custom", "":
		if strings.TrimSpace(cfg.WebhookURL) == "" {
			return fmt.Errorf("webhook_url is empty")
		}
	case "bark":
		if strings.TrimSpace(cfg.BarkToken) == "" {
			return fmt.Errorf("bark_token is empty")
		}
	case "wecom_app", "qywx", "qywx_app":
		if cfg.QywxCorpID == "" || cfg.QywxCorpSecret == "" || cfg.QywxAgentID == "" {
			return fmt.Errorf("qywx corpid/agentid/corpsecret required")
		}
	}
	return nil
}

type Noop struct{}

func (n *Noop) Send(ctx context.Context, msg Message) error { return nil }

// DingTalk sends markdown via DingTalk robot webhook.
type DingTalk struct{ URL string }

func (d *DingTalk) Send(ctx context.Context, msg Message) error {
	body := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": msg.Title,
			"text":  fmt.Sprintf("### %s\n\n%s", msg.Title, msg.Content),
		},
	}
	return postJSON(ctx, d.URL, body)
}

// WeComBot sends text via WeCom robot webhook.
type WeComBot struct{ URL string }

func (w *WeComBot) Send(ctx context.Context, msg Message) error {
	body := map[string]any{
		"msgtype": "text",
		"text": map[string]string{
			"content": fmt.Sprintf("%s\n%s", msg.Title, msg.Content),
		},
	}
	return postJSON(ctx, w.URL, body)
}

// WeComApp sends text via WeCom application API (token cached).
type WeComApp struct {
	CorpID     string
	CorpSecret string
	AgentID    string
	ToUser     string

	mu      sync.Mutex
	token   string
	expires time.Time
}

func (w *WeComApp) Send(ctx context.Context, msg Message) error {
	token, err := w.accessToken(ctx)
	if err != nil {
		return err
	}
	touser := w.ToUser
	if touser == "" {
		touser = "@all"
	}
	agentID := json.Number(w.AgentID)
	// agentid may be numeric string
	var agent any = w.AgentID
	if n, e := agentID.Int64(); e == nil {
		agent = n
	}
	body := map[string]any{
		"touser":  touser,
		"msgtype": "text",
		"agentid": agent,
		"text": map[string]string{
			"content": fmt.Sprintf("【%s】\n%s", msg.Title, msg.Content),
		},
		"safe": 0,
	}
	url := "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=" + token
	return postJSON(ctx, url, body)
}

func (w *WeComApp) accessToken(ctx context.Context) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.token != "" && time.Now().Before(w.expires) {
		return w.token, nil
	}
	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		w.CorpID, w.CorpSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := sharedHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if out.ErrCode != 0 || out.AccessToken == "" {
		return "", fmt.Errorf("qywx token: %d %s", out.ErrCode, out.ErrMsg)
	}
	w.token = out.AccessToken
	sec := out.ExpiresIn
	if sec <= 0 {
		sec = 7200
	}
	// refresh a bit early
	w.expires = time.Now().Add(time.Duration(sec-120) * time.Second)
	return w.token, nil
}

// Bark pushes to Bark server /push API.
type Bark struct {
	Server string
	Token  string
	Sound  string
	Icon   string
	Level  string
	Group  string
}

func (b *Bark) Send(ctx context.Context, msg Message) error {
	payload := map[string]any{
		"device_key": b.Token,
		"title":      msg.Title,
		"body":       msg.Content,
		"isArchive":  1,
	}
	if b.Sound != "" {
		payload["sound"] = b.Sound
	}
	ext := map[string]any{}
	if b.Group != "" {
		ext["group"] = b.Group
	}
	if b.Icon != "" {
		ext["icon"] = b.Icon
	}
	if b.Level != "" {
		ext["level"] = b.Level
	}
	if len(ext) > 0 {
		payload["ext_params"] = ext
	}
	return postJSON(ctx, b.Server+"/push", payload)
}

// Webhook posts generic JSON.
type Webhook struct {
	URL      string
	Template string
}

func (w *Webhook) Send(ctx context.Context, msg Message) error {
	if w.Template != "" {
		s := w.Template
		s = strings.ReplaceAll(s, "{{title}}", escapeJSON(msg.Title))
		s = strings.ReplaceAll(s, "{{content}}", escapeJSON(msg.Content))
		return postRaw(ctx, w.URL, []byte(s))
	}
	return postJSON(ctx, w.URL, map[string]string{
		"title":   msg.Title,
		"content": msg.Content,
	})
}

func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	if len(b) >= 2 {
		return string(b[1 : len(b)-1])
	}
	return s
}

func postJSON(ctx context.Context, url string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return postRaw(ctx, url, b)
}

func postRaw(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := sharedHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	return nil
}

// FormatDeviceOpts optional fields for device event messages.
type FormatDeviceOpts struct {
	OnlineDuration string // set for 下线
	OnlineList     string // preformatted list block (may be empty)
}

// FormatDevice builds title/content for online/offline.
func FormatDevice(deviceName, action string, mac, ip, name, iface string, opts FormatDeviceOpts) Message {
	title := fmt.Sprintf("[%s] 设备%s", deviceName, action)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("名称: %s\nMAC: %s\nIP: %s\n接口: %s\n", truncateDisplay(name, nameColWidth), mac, ip, iface))
	if action == "下线" && opts.OnlineDuration != "" {
		b.WriteString("在线时长: ")
		b.WriteString(opts.OnlineDuration)
		b.WriteByte('\n')
	}
	b.WriteString("时间: ")
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	if opts.OnlineList != "" {
		b.WriteByte('\n')
		b.WriteByte('\n')
		b.WriteString(opts.OnlineList)
	}
	return Message{Title: title, Content: b.String()}
}

// OnlineListItem is one row in the online device table.
type OnlineListItem struct {
	Name        string
	IP          string
	OnlineSince time.Time
}

type onlineListRow struct {
	item OnlineListItem
	dur  time.Duration
}

const (
	ipColWidth   = 11 // fits 10.0.0.255
	durColWidth  = 4  // e.g. 46d, 12h, 5m
	nameColWidth = 10 // truncate long DHCP/OUI names for compact one-line rows
)

// FormatOnlineList builds an aligned online-device table.
// Columns: IP | duration (fixed width) | name. Sort: short → long uptime.
func FormatOnlineList(items []OnlineListItem, now time.Time, max int) string {
	if max <= 0 {
		max = 15
	}
	rows := make([]onlineListRow, 0, len(items))
	for _, it := range items {
		since := it.OnlineSince
		if since.IsZero() {
			// Unknown start → show 0m rather than inventing from last_seen.
			rows = append(rows, onlineListRow{item: it, dur: 0})
			continue
		}
		rows = append(rows, onlineListRow{item: it, dur: now.Sub(since)})
	}
	rows = dedupeRowsByIP(rows)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].dur == rows[j].dur {
			return rows[i].item.IP < rows[j].item.IP
		}
		return rows[i].dur < rows[j].dur
	})

	total := len(rows)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("在线设备 (%d):\n", total))
	b.WriteString(padDisplay("IP", ipColWidth))
	b.WriteByte(' ')
	b.WriteString(padDisplay("时长", durColWidth))
	b.WriteByte(' ')
	b.WriteString(fitDisplay("名称", nameColWidth))
	b.WriteByte('\n')

	limit := total
	if limit > max {
		limit = max
	}
	for i := 0; i < limit; i++ {
		r := rows[i]
		name := r.item.Name
		if name == "" {
			name = "unknown"
		}
		b.WriteString(padDisplay(r.item.IP, ipColWidth))
		b.WriteByte(' ')
		b.WriteString(padDisplay(FormatDuration(r.dur), durColWidth))
		b.WriteByte(' ')
		b.WriteString(fitDisplay(name, nameColWidth))
		b.WriteByte('\n')
	}
	if total > max {
		b.WriteString(fmt.Sprintf("… 共%d台\n", total))
	}
	return strings.TrimRight(b.String(), "\n")
}

// FormatDuration renders a single unit: Xd, Xh, or Xm (largest applicable).
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalMin := int(d / time.Minute)
	if totalMin >= 60*24 {
		return fmt.Sprintf("%dd", totalMin/(60*24))
	}
	if totalMin >= 60 {
		return fmt.Sprintf("%dh", totalMin/60)
	}
	return fmt.Sprintf("%dm", totalMin)
}

func isUnknownName(name string) bool {
	return name == "" || name == "unknown"
}

func dedupeRowsByIP(rows []onlineListRow) []onlineListRow {
	if len(rows) <= 1 {
		return rows
	}
	best := make(map[string]onlineListRow, len(rows))
	for _, r := range rows {
		ip := r.item.IP
		if ip == "" {
			continue
		}
		prev, ok := best[ip]
		if !ok {
			best[ip] = r
			continue
		}
		if isUnknownName(prev.item.Name) && !isUnknownName(r.item.Name) {
			best[ip] = r
			continue
		}
		if !isUnknownName(prev.item.Name) && isUnknownName(r.item.Name) {
			continue
		}
		if r.dur > prev.dur {
			best[ip] = r
		}
	}
	out := make([]onlineListRow, 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	return out
}

func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r <= 127 {
			w++
		} else {
			w += 2
		}
	}
	return w
}

func padDisplay(s string, width int) string {
	w := displayWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func truncateDisplay(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(s) <= width {
		return s
	}
	const suffix = "…"
	suffixW := displayWidth(suffix)
	budget := width - suffixW
	if budget <= 0 {
		return suffix
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := 1
		if r > 127 {
			rw = 2
		}
		if w+rw > budget {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + suffix
}

func fitDisplay(s string, width int) string {
	return padDisplay(truncateDisplay(s, width), width)
}
