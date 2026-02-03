package netmgr

import (
	"errors"
	"sync"
)

type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateOnline   State = "online"
	StateDegraded State = "degraded"
	StateStopping State = "stopping"
)

var ErrInvalidTransition = errors.New("invalid net state transition")

type Manager struct {
	mu      sync.Mutex
	state   State
	reasons map[string]bool
}

func New() *Manager {
	return &Manager{
		state:   StateStopped,
		reasons: map[string]bool{},
	}
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *Manager) Reasons() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.reasons))
	for r := range m.reasons {
		out = append(out, r)
	}
	return out
}

func (m *Manager) Transition(next State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !validTransition(m.state, next) {
		return ErrInvalidTransition
	}
	m.state = next
	return nil
}

func (m *Manager) AddDegradedReason(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if reason != "" {
		m.reasons[reason] = true
	}
	if len(m.reasons) > 0 && (m.state == StateOnline || m.state == StateDegraded) {
		m.state = StateDegraded
	}
}

func (m *Manager) ClearDegradedReason(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.reasons, reason)
	if len(m.reasons) == 0 && m.state == StateDegraded {
		m.state = StateOnline
	}
}

func validTransition(from, to State) bool {
	switch from {
	case StateStopped:
		return to == StateStarting
	case StateStarting:
		return to == StateOnline || to == StateDegraded || to == StateStopping
	case StateOnline:
		return to == StateDegraded || to == StateStopping
	case StateDegraded:
		return to == StateOnline || to == StateStopping
	case StateStopping:
		return to == StateStopped
	default:
		return false
	}
}
