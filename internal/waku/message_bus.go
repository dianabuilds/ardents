package waku

import "sync"

type PrivateMessage struct {
	ID        string
	SenderID  string
	Recipient string
	Payload   []byte
}

type messageBus struct {
	mu          sync.Mutex
	subscribers map[string]func(PrivateMessage)
	mailbox     map[string][]PrivateMessage
}

var globalBus = &messageBus{
	subscribers: make(map[string]func(PrivateMessage)),
	mailbox:     make(map[string][]PrivateMessage),
}

func (b *messageBus) publish(msg PrivateMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if handler, ok := b.subscribers[msg.Recipient]; ok {
		go handler(msg)
		return
	}
	b.mailbox[msg.Recipient] = append(b.mailbox[msg.Recipient], msg)
}

func (b *messageBus) subscribe(recipient string, handler func(PrivateMessage)) {
	b.mu.Lock()
	b.subscribers[recipient] = handler
	pending := append([]PrivateMessage(nil), b.mailbox[recipient]...)
	delete(b.mailbox, recipient)
	b.mu.Unlock()

	for _, msg := range pending {
		handler(msg)
	}
}

func (b *messageBus) unsubscribe(recipient string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, recipient)
}
