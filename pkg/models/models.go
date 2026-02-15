package models

import "time"

type Account struct {
	ID                string `json:"id"`
	IdentityPublicKey []byte `json:"identity_public_key"`
}

type Identity struct {
	ID               string `json:"id"`
	SigningPublicKey []byte `json:"signing_public_key"`
}

type ContactCard struct {
	IdentityID  string `json:"identity_id"`
	DisplayName string `json:"display_name"`
	PublicKey   []byte `json:"public_key"`
	Signature   []byte `json:"signature"`
}

type Contact struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	PublicKey   []byte    `json:"public_key"`
	AddedAt     time.Time `json:"added_at"`
	LastSeen    time.Time `json:"last_seen"`
}

type Message struct {
	ID          string    `json:"id"`
	ContactID   string    `json:"contact_id"`
	Content     []byte    `json:"content"`
	Timestamp   time.Time `json:"timestamp"`
	Direction   string    `json:"direction"`
	Status      string    `json:"status"`
	ContentType string    `json:"content_type"`
	Edited      bool      `json:"edited"`
}

type Settings struct {
	Theme          string `json:"theme"`
	Language       string `json:"language"`
	Notifications  bool   `json:"notifications"`
	AutoStart      bool   `json:"auto_start"`
	EnablePresence bool   `json:"enable_presence"`
}

type NetworkStatus struct {
	Status    string    `json:"status"`
	PeerCount int       `json:"peer_count"`
	LastSync  time.Time `json:"last_sync"`
}

type SessionState struct {
	SessionID      string    `json:"session_id"`
	ContactID      string    `json:"contact_id"`
	PeerPublicKey  []byte    `json:"peer_public_key"`
	SendChainIndex uint64    `json:"send_chain_index"`
	RecvChainIndex uint64    `json:"recv_chain_index"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type MetricsSnapshot struct {
	PeerCount           int                        `json:"peer_count"`
	PendingQueueSize    int                        `json:"pending_queue_size"`
	ErrorCounters       map[string]int             `json:"error_counters"`
	NetworkMetrics      map[string]int             `json:"network_metrics"`
	OperationStats      map[string]OperationMetric `json:"operation_stats"`
	RetryAttemptsTotal  int                        `json:"retry_attempts_total"`
	LastUpdatedAt       time.Time                  `json:"last_updated_at"`
	NotificationBacklog int                        `json:"notification_backlog"`
}

type OperationMetric struct {
	Count         int   `json:"count"`
	Errors        int   `json:"errors"`
	AvgLatencyMs  int64 `json:"avg_latency_ms"`
	MaxLatencyMs  int64 `json:"max_latency_ms"`
	LastLatencyMs int64 `json:"last_latency_ms"`
}

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	PublicKey []byte    `json:"public_key"`
	CertSig   []byte    `json:"cert_sig"`
	CreatedAt time.Time `json:"created_at"`
	IsRevoked bool      `json:"is_revoked"`
	RevokedAt time.Time `json:"revoked_at,omitempty"`
}

type DeviceRevocation struct {
	IdentityID string    `json:"identity_id"`
	DeviceID   string    `json:"device_id"`
	Timestamp  time.Time `json:"timestamp"`
	Signature  []byte    `json:"signature"`
}

type MessageReceipt struct {
	MessageID string    `json:"message_id"`
	Status    string    `json:"status"` // delivered, read
	Timestamp time.Time `json:"timestamp"`
}

type MessageStatus struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

type AttachmentMeta struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	MimeType  string    `json:"mime_type"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}
