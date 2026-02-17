package group

import (
	"sync"
	"time"
)

// RuntimeState owns in-memory group runtime state (membership snapshot + replay guard cache).
type RuntimeState struct {
	StateMu    *sync.RWMutex
	States     map[string]GroupState
	EventLog   map[string][]GroupEvent
	ReplayMu   *sync.Mutex
	ReplaySeen map[string]time.Time
}

func NewRuntimeState() *RuntimeState {
	return &RuntimeState{
		StateMu:    &sync.RWMutex{},
		States:     make(map[string]GroupState),
		EventLog:   make(map[string][]GroupEvent),
		ReplayMu:   &sync.Mutex{},
		ReplaySeen: make(map[string]time.Time),
	}
}

func (r *RuntimeState) SetSnapshot(states map[string]GroupState, eventLog map[string][]GroupEvent) {
	if r == nil {
		return
	}
	if states == nil {
		states = make(map[string]GroupState)
	}
	if eventLog == nil {
		eventLog = make(map[string][]GroupEvent)
	}
	r.States = states
	r.EventLog = eventLog
}
