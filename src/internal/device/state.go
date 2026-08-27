package device

import (
	"sync"
	"time"
)

// State is the per-MAC lifecycle state.
type State string

const (
	StatePendingUp State = "pending_up"
	StateOnline    State = "online"
	StateSuspect   State = "suspect"
	StateOffline   State = "offline"
)

// EventKind classifies inputs into the state machine.
type EventKind string

const (
	EventStrongUp  EventKind = "strong_up" // REACHABLE / NEW / DHCP lease
	EventWeakSeen  EventKind = "weak_seen" // STALE / DELAY — do NOT go offline
	EventFailed    EventKind = "failed"    // FAILED / DELETE
	EventProbeOK   EventKind = "probe_ok"
	EventProbeFail EventKind = "probe_fail"
	EventTimeout   EventKind = "timeout" // suspect timeout tick
)

// Device is one LAN host keyed by MAC.
type Device struct {
	MAC         string    `json:"mac"`
	IP          string    `json:"ip"`
	Name        string    `json:"name"`
	Iface       string    `json:"iface"`
	State       State     `json:"state"`
	FailCount   int       `json:"fail_count"`
	ProbeIndex  int       `json:"-"` // backoff index
	LastSeen    time.Time `json:"last_seen"`
	OnlineSince time.Time `json:"online_since,omitempty"`
	SuspectAt   time.Time `json:"suspect_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
	NotifiedUp  bool      `json:"-"`
	NotifiedOff bool      `json:"-"`
}

// Transition is the result of applying an event.
type Transition struct {
	Device        *Device
	BecameOnline  bool // first confirmed online → notify
	BecameOffline bool
	NeedProbe     bool
}

// Params tunes offline detection.
type Params struct {
	OfflineFailCount  int
	SuspectTimeoutSec int
}

var backoffSchedule = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second}

// NextBackoff returns probe delay for the given fail index.
func NextBackoff(index int) time.Duration {
	if index < 0 {
		index = 0
	}
	if index >= len(backoffSchedule) {
		return backoffSchedule[len(backoffSchedule)-1]
	}
	return backoffSchedule[index]
}

// ApplyEvent mutates d according to the event and returns transition flags.
// STALE (EventWeakSeen) never forces offline.
func ApplyEvent(d *Device, kind EventKind, p Params, now time.Time) Transition {
	tr := Transition{Device: d}
	if d == nil {
		return tr
	}
	d.UpdatedAt = now

	switch kind {
	case EventStrongUp, EventProbeOK:
		d.State = StateOnline
		d.FailCount = 0
		d.ProbeIndex = 0
		d.SuspectAt = time.Time{}
		d.LastSeen = now
		// First confirmed online (incl. after offline). Suspect recovery with
		// NotifiedUp already true must not re-notify.
		if !d.NotifiedUp {
			tr.BecameOnline = true
			d.NotifiedUp = true
			d.NotifiedOff = false
			d.OnlineSince = now
		}

	case EventWeakSeen:
		// STALE/DELAY/PROBE: never force offline. Soft-confirm pending_up → online
		// so we do not depend on ARP when neighbour table already knows the host.
		d.LastSeen = now
		switch d.State {
		case StateOnline:
			// refresh only
		case StateSuspect:
			d.State = StateOnline
			d.FailCount = 0
			d.ProbeIndex = 0
			d.SuspectAt = time.Time{}
		case StatePendingUp, StateOffline, "":
			d.State = StateOnline
			d.FailCount = 0
			d.ProbeIndex = 0
			d.SuspectAt = time.Time{}
			if !d.NotifiedUp {
				tr.BecameOnline = true
				d.NotifiedUp = true
				d.NotifiedOff = false
				d.OnlineSince = now
			}
		}

	case EventFailed:
		if d.State == StateOnline || d.State == StatePendingUp {
			d.State = StateSuspect
			d.SuspectAt = now
			d.FailCount = 0
			d.ProbeIndex = 0
			tr.NeedProbe = true
		} else if d.State == StateSuspect {
			tr.NeedProbe = true
		}

	case EventProbeFail:
		if d.State != StateSuspect && d.State != StatePendingUp {
			return tr
		}
		d.FailCount++
		d.ProbeIndex++
		if d.FailCount >= p.OfflineFailCount {
			return goOffline(d, &tr)
		}
		tr.NeedProbe = true

	case EventTimeout:
		// Keep pending_up probing alive if a prior schedule was lost.
		if d.State == StatePendingUp {
			tr.NeedProbe = true
			return tr
		}
		if d.State != StateSuspect {
			return tr
		}
		if p.SuspectTimeoutSec > 0 && !d.SuspectAt.IsZero() {
			if now.Sub(d.SuspectAt) >= time.Duration(p.SuspectTimeoutSec)*time.Second {
				return goOffline(d, &tr)
			}
		}
		tr.NeedProbe = true
	}
	return tr
}

func goOffline(d *Device, tr *Transition) Transition {
	d.State = StateOffline
	d.SuspectAt = time.Time{}
	if d.NotifiedUp && !d.NotifiedOff {
		tr.BecameOffline = true
		d.NotifiedOff = true
		d.NotifiedUp = false
	}
	return *tr
}

// Store is a thread-safe MAC → Device map.
type Store struct {
	mu   sync.RWMutex
	devs map[string]*Device
}

func NewStore() *Store {
	return &Store{devs: make(map[string]*Device)}
}

func (s *Store) GetOrCreate(mac string) *Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getOrCreateLocked(mac)
}

// GetOrCreateUnsafe requires Lock() held by caller.
func (s *Store) GetOrCreateUnsafe(mac string) *Device {
	return s.getOrCreateLocked(mac)
}

func (s *Store) getOrCreateLocked(mac string) *Device {
	if d, ok := s.devs[mac]; ok {
		return d
	}
	d := &Device{MAC: mac, State: StatePendingUp, UpdatedAt: time.Now()}
	s.devs[mac] = d
	return d
}

func (s *Store) Get(mac string) (*Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devs[mac]
	return d, ok
}

func (s *Store) Update(mac string, fn func(*Device)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreateLocked(mac)
	fn(d)
}

func (s *Store) Snapshot() []Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Device, 0, len(s.devs))
	for _, d := range s.devs {
		out = append(out, *d)
	}
	return out
}

func (s *Store) Lock()   { s.mu.Lock() }
func (s *Store) Unlock() { s.mu.Unlock() }
