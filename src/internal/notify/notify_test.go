package notify

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zouyq/netnotify/internal/config"
)

func TestWebhookTemplateSubstitution(t *testing.T) {
	tpl := `{"title":"{{title}}","content":"{{content}}"}`
	msg := Message{Title: `hi "x"`, Content: "line1\nline2"}
	s := tpl
	s = strings.ReplaceAll(s, "{{title}}", escapeJSON(msg.Title))
	s = strings.ReplaceAll(s, "{{content}}", escapeJSON(msg.Content))
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err, s)
	}
	if m["title"] != msg.Title {
		t.Fatalf("title %q", m["title"])
	}
}

func TestFormatDevice(t *testing.T) {
	msg := FormatDevice("Router", "上线", "aa:bb:cc:dd:ee:ff", "192.168.1.2", "Phone", "br-lan", FormatDeviceOpts{})
	if !strings.Contains(msg.Title, "上线") {
		t.Fatal(msg.Title)
	}
	if !strings.Contains(msg.Content, "aa:bb:cc:dd:ee:ff") {
		t.Fatal(msg.Content)
	}
}

func TestFormatDeviceOfflineWithList(t *testing.T) {
	now := time.Date(2026, 8, 27, 20, 30, 0, 0, time.Local)
	list := FormatOnlineList([]OnlineListItem{
		{Name: "iPhone", IP: "10.0.0.51", OnlineSince: now.Add(-12 * time.Minute)},
		{Name: "home-as", IP: "10.0.0.20", OnlineSince: now.Add(-27 * time.Hour)},
		{Name: "midea_da_0270", IP: "10.0.0.34", OnlineSince: now.Add(-5*time.Hour - 20*time.Minute)},
	}, now, 15)
	msg := FormatDevice("home-jd", "下线", "64:41:e6:b9:bc:23", "10.0.0.18", "wangxiadeiPhone", "br-lan", FormatDeviceOpts{
		OnlineDuration: "2小时15分",
		OnlineList:     list,
	})
	if !strings.Contains(msg.Content, "在线时长: 2小时15分") {
		t.Fatal(msg.Content)
	}
	if !strings.Contains(msg.Content, "当前在线 (3):") {
		t.Fatal(msg.Content)
	}
	if !strings.Contains(list, "IP") || !strings.Contains(list, "时长") {
		t.Fatal(list)
	}
	// Column order: IP first on data rows
	lines := strings.Split(list, "\n")
	if len(lines) < 3 || !strings.HasPrefix(strings.TrimSpace(lines[2]), "10.0.0.") {
		t.Fatalf("expect IP-first rows:\n%s", list)
	}
	// short → long: iPhone before home-as
	idxPhone := strings.Index(list, "iPhone")
	idxHome := strings.Index(list, "home-as")
	if idxPhone < 0 || idxHome < 0 || idxPhone > idxHome {
		t.Fatalf("sort short→long failed:\n%s", list)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		0:                             "0分",
		5 * time.Minute:               "5分",
		65 * time.Minute:              "1小时5分",
		2 * time.Hour:                 "2小时",
		26*time.Hour + 10*time.Minute: "1天2小时",
	}
	for d, want := range cases {
		if got := FormatDuration(d); got != want {
			t.Fatalf("%v: got %s want %s", d, got, want)
		}
	}
}

func TestFormatOnlineListMax(t *testing.T) {
	now := time.Now()
	items := make([]OnlineListItem, 0, 18)
	for i := 0; i < 18; i++ {
		items = append(items, OnlineListItem{
			Name:        "dev",
			IP:          fmt.Sprintf("10.0.0.%d", i+1),
			OnlineSince: now.Add(-time.Duration(i) * time.Minute),
		})
	}
	s := FormatOnlineList(items, now, 15)
	if !strings.Contains(s, "当前在线 (18):") {
		t.Fatal(s)
	}
	if !strings.Contains(s, "… 共18台") {
		t.Fatal(s)
	}
}

func TestNewChannels(t *testing.T) {
	for _, ch := range []string{"dingtalk", "wecom_bot", "webhook", "bark", "wecom_app"} {
		cfg := config.Config{Channel: ch, WebhookURL: "http://127.0.0.1/hook", BarkToken: "k", QywxCorpID: "c", QywxCorpSecret: "s", QywxAgentID: "1"}
		s, err := FromConfig(cfg)
		if err != nil || s == nil {
			t.Fatalf("%s: %v", ch, err)
		}
	}
	s, _ := FromConfig(config.Config{Channel: "webhook"})
	if _, ok := s.(*Noop); !ok {
		t.Fatal("empty url should be noop")
	}
}
