package usecase

type InboundPrivateMessage struct {
	ID        string
	SenderID  string
	Recipient string
	Payload   []byte
}
