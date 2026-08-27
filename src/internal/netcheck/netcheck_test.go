package netcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckGenerate204(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/generate_204" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	c := New()
	ok, via, _ := c.CheckHosts(context.Background(), Params{
		Hosts:      []string{host},
		Retry:      1,
		TimeoutSec: 2,
	})
	if !ok || via != host {
		t.Fatalf("ok=%v via=%s", ok, via)
	}
}

func TestFormatUptime(t *testing.T) {
	if FormatUptime(90) != "1分" {
		t.Fatal(FormatUptime(90))
	}
	if FormatUptime(3700) != "1小时1分" {
		t.Fatal(FormatUptime(3700))
	}
}
