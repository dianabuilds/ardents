package inbox

import (
	"sync"

	"aim-chat/go-backend/pkg/models"
)

// RuntimeState owns in-memory message-request inbox runtime state.
type RuntimeState struct {
	Mu    *sync.RWMutex
	Inbox map[string][]models.Message
}

func NewRuntimeState() *RuntimeState {
	return &RuntimeState{
		Mu:    &sync.RWMutex{},
		Inbox: make(map[string][]models.Message),
	}
}

func (r *RuntimeState) SetInbox(inbox map[string][]models.Message) {
	if r == nil {
		return
	}
	if inbox == nil {
		inbox = make(map[string][]models.Message)
	}
	r.Inbox = inbox
}
