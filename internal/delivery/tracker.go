package delivery

import (
	"sync"
)

type Status string

const (
	StatusSent     Status = "sent"
	StatusAcked    Status = "acked"
	StatusFailed   Status = "failed"
	StatusRejected Status = "rejected"
)

type Record struct {
	MsgID     string
	Status    Status
	ErrorCode string
}

type Tracker struct {
	mu      sync.Mutex
	records map[string]Record
}

func NewTracker() *Tracker {
	return &Tracker{records: make(map[string]Record)}
}

func (t *Tracker) Set(rec Record) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records[rec.MsgID] = rec
}

func (t *Tracker) Get(msgID string) (Record, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.records[msgID]
	return rec, ok
}

func (t *Tracker) LastID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var id string
	for k := range t.records {
		id = k
	}
	return id
}
