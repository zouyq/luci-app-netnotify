package device

import (
	"testing"
	"time"
)

func TestSTALEDoesNotOffline(t *testing.T) {
	d := &Device{MAC: "aa:bb:cc:dd:ee:ff", State: StateOnline, NotifiedUp: true}
	p := Params{OfflineFailCount: 3, SuspectTimeoutSec: 60}
	now := time.Now()
	tr := ApplyEvent(d, EventWeakSeen, p, now)
	if d.State != StateOnline {
		t.Fatalf("STALE must not change online→offline, got %s", d.State)
	}
	if tr.BecameOffline {
		t.Fatal("STALE must not notify offline")
	}
}

func TestWeakSeenPromotesPendingUp(t *testing.T) {
	d := &Device{MAC: "aa:bb:cc:dd:ee:08", State: StatePendingUp}
	p := Params{OfflineFailCount: 3, SuspectTimeoutSec: 60}
	tr := ApplyEvent(d, EventWeakSeen, p, time.Now())
	if d.State != StateOnline {
		t.Fatalf("want online from pending via STALE, got %s", d.State)
	}
	if !tr.BecameOnline {
		t.Fatal("first soft-confirm should notify online")
	}
}

func TestPendingUpProbeOK(t *testing.T) {
	d := &Device{MAC: "aa:bb:cc:dd:ee:01", State: StatePendingUp}
	p := Params{OfflineFailCount: 3, SuspectTimeoutSec: 60}
	tr := ApplyEvent(d, EventProbeOK, p, time.Now())
	if d.State != StateOnline {
		t.Fatalf("want online, got %s", d.State)
	}
	if !tr.BecameOnline {
		t.Fatal("first online should notify")
	}
}

func TestSuspectToOfflineByFails(t *testing.T) {
	d := &Device{MAC: "aa:bb:cc:dd:ee:02", State: StateSuspect, NotifiedUp: true, SuspectAt: time.Now()}
	p := Params{OfflineFailCount: 3, SuspectTimeoutSec: 60}
	now := time.Now()
	for i := 0; i < 2; i++ {
		tr := ApplyEvent(d, EventProbeFail, p, now)
		if tr.BecameOffline || d.State == StateOffline {
			t.Fatalf("should not offline before N fails, i=%d state=%s", i, d.State)
		}
		if !tr.NeedProbe {
			t.Fatal("should need another probe")
		}
	}
	tr := ApplyEvent(d, EventProbeFail, p, now)
	if d.State != StateOffline {
		t.Fatalf("want offline, got %s", d.State)
	}
	if !tr.BecameOffline {
		t.Fatal("should notify offline")
	}
}

func TestSuspectTimeout(t *testing.T) {
	start := time.Now()
	d := &Device{MAC: "aa:bb:cc:dd:ee:03", State: StateSuspect, NotifiedUp: true, SuspectAt: start}
	p := Params{OfflineFailCount: 99, SuspectTimeoutSec: 60}
	tr := ApplyEvent(d, EventTimeout, p, start.Add(30*time.Second))
	if d.State != StateSuspect {
		t.Fatalf("too early for timeout, got %s", d.State)
	}
	_ = tr
	tr = ApplyEvent(d, EventTimeout, p, start.Add(61*time.Second))
	if d.State != StateOffline || !tr.BecameOffline {
		t.Fatalf("timeout should offline+notify, state=%s offline=%v", d.State, tr.BecameOffline)
	}
}

func TestFailedMovesOnlineToSuspect(t *testing.T) {
	d := &Device{MAC: "aa:bb:cc:dd:ee:04", State: StateOnline, NotifiedUp: true}
	p := Params{OfflineFailCount: 3, SuspectTimeoutSec: 60}
	tr := ApplyEvent(d, EventFailed, p, time.Now())
	if d.State != StateSuspect {
		t.Fatalf("want suspect, got %s", d.State)
	}
	if !tr.NeedProbe {
		t.Fatal("suspect should need probe")
	}
	if tr.BecameOffline {
		t.Fatal("must not offline immediately on FAILED")
	}
}

func TestSuspectProbeOKBackOnline(t *testing.T) {
	d := &Device{MAC: "aa:bb:cc:dd:ee:05", State: StateSuspect, NotifiedUp: true, FailCount: 2}
	p := Params{OfflineFailCount: 3, SuspectTimeoutSec: 60}
	tr := ApplyEvent(d, EventProbeOK, p, time.Now())
	if d.State != StateOnline {
		t.Fatalf("want online, got %s", d.State)
	}
	if d.FailCount != 0 {
		t.Fatal("fail count should reset")
	}
	// already notified up — no duplicate online notify unless from offline
	_ = tr
}

func TestPendingUpTimeoutNeedsProbe(t *testing.T) {
	d := &Device{MAC: "aa:bb:cc:dd:ee:07", State: StatePendingUp}
	p := Params{OfflineFailCount: 3, SuspectTimeoutSec: 60}
	tr := ApplyEvent(d, EventTimeout, p, time.Now())
	if d.State != StatePendingUp {
		t.Fatalf("pending_up must stay pending, got %s", d.State)
	}
	if !tr.NeedProbe {
		t.Fatal("pending_up timeout tick should re-arm probe")
	}
}

func TestNextBackoff(t *testing.T) {
	if NextBackoff(0) != 2*time.Second {
		t.Fatal(NextBackoff(0))
	}
	if NextBackoff(3) != 30*time.Second {
		t.Fatal(NextBackoff(3))
	}
	if NextBackoff(99) != 30*time.Second {
		t.Fatal(NextBackoff(99))
	}
}

func TestReOnlineAfterOfflineNotifies(t *testing.T) {
	d := &Device{MAC: "aa:bb:cc:dd:ee:06", State: StateOffline, NotifiedUp: false, NotifiedOff: true}
	p := Params{OfflineFailCount: 3, SuspectTimeoutSec: 60}
	tr := ApplyEvent(d, EventStrongUp, p, time.Now())
	if !tr.BecameOnline {
		t.Fatal("re-online after offline should notify")
	}
	if d.State != StateOnline {
		t.Fatalf("got %s", d.State)
	}
}

func TestRestoreKeepsOnlineSinceAndSuppressesRenotify(t *testing.T) {
	since := time.Now().Add(-3 * time.Hour)
	s := NewStore()
	n := s.Restore([]Device{{
		MAC:         "aa:bb:cc:dd:ee:10",
		IP:          "10.0.0.10",
		Name:        "phone",
		State:       StateOnline,
		OnlineSince: since,
		LastSeen:    time.Now(),
	}})
	if n != 1 {
		t.Fatalf("restored %d", n)
	}
	d, ok := s.Get("aa:bb:cc:dd:ee:10")
	if !ok || !d.NotifiedUp || !d.OnlineSince.Equal(since) {
		t.Fatalf("bad restore: %+v", d)
	}
	tr := ApplyEvent(d, EventWeakSeen, Params{OfflineFailCount: 3, SuspectTimeoutSec: 60}, time.Now())
	if tr.BecameOnline {
		t.Fatal("restored online must not re-notify")
	}
	if !d.OnlineSince.Equal(since) {
		t.Fatal("OnlineSince must not reset on weak seen")
	}
}
