package models

import (
	"strings"
	"time"
)

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
	ID               string    `json:"id"`
	ContactID        string    `json:"contact_id"`
	ConversationID   string    `json:"conversation_id,omitempty"`
	ConversationType string    `json:"conversation_type,omitempty"`
	ThreadID         string    `json:"thread_id,omitempty"`
	Content          []byte    `json:"content"`
	Timestamp        time.Time `json:"timestamp"`
	Direction        string    `json:"direction"`
	Status           string    `json:"status"`
	ContentType      string    `json:"content_type"`
	Edited           bool      `json:"edited"`
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
	PeerCount              int                        `json:"peer_count"`
	PendingQueueSize       int                        `json:"pending_queue_size"`
	ErrorCounters          map[string]int             `json:"error_counters"`
	GroupAggregates        map[string]int             `json:"group_aggregates,omitempty"`
	NetworkMetrics         map[string]int             `json:"network_metrics"`
	DiskUsageByClass       map[string]int64           `json:"disk_usage_by_class,omitempty"`
	GCEvictionCountByClass map[string]int             `json:"gc_eviction_count_by_class,omitempty"`
	BlobFetchStats         BlobFetchMetric            `json:"blob_fetch_stats,omitempty"`
	StorageGuardrails      map[string]int             `json:"storage_guardrails,omitempty"`
	OperationStats         map[string]OperationMetric `json:"operation_stats"`
	RetryAttemptsTotal     int                        `json:"retry_attempts_total"`
	LastUpdatedAt          time.Time                  `json:"last_updated_at"`
	NotificationBacklog    int                        `json:"notification_backlog"`
}

type OperationMetric struct {
	Count         int   `json:"count"`
	Errors        int   `json:"errors"`
	AvgLatencyMs  int64 `json:"avg_latency_ms"`
	MaxLatencyMs  int64 `json:"max_latency_ms"`
	LastLatencyMs int64 `json:"last_latency_ms"`
}

type BlobFetchMetric struct {
	AttemptsTotal      int            `json:"attempts_total"`
	SuccessTotal       int            `json:"success_total"`
	UnavailableTotal   int            `json:"unavailable_total"`
	FailureTotal       int            `json:"failure_total"`
	UnavailableReasons map[string]int `json:"unavailable_reasons,omitempty"`
	AvgLatencyMs       int64          `json:"avg_latency_ms"`
	MaxLatencyMs       int64          `json:"max_latency_ms"`
	LastLatencyMs      int64          `json:"last_latency_ms"`
	UnavailableRateBps int64          `json:"unavailable_rate_bps"`
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

type AttachmentClass string

const (
	AttachmentClassImage AttachmentClass = "image"
	AttachmentClassFile  AttachmentClass = "file"
)

type AttachmentPinState string

const (
	AttachmentPinStateUnpinned AttachmentPinState = "unpinned"
	AttachmentPinStatePinned   AttachmentPinState = "pinned"
)

type AttachmentMeta struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	MimeType     string    `json:"mime_type"`
	Class        string    `json:"class,omitempty"`
	LastAccessAt time.Time `json:"last_access_at,omitempty"`
	PinState     string    `json:"pin_state,omitempty"`
	Size         int64     `json:"size"`
	CreatedAt    time.Time `json:"created_at"`
}

type BlobProviderInfo struct {
	PeerID    string    `json:"peer_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type BlobFeatureFlags struct {
	AnnounceEnabled bool `json:"announce_enabled"`
	FetchEnabled    bool `json:"fetch_enabled"`
	RolloutPercent  int  `json:"rollout_percent"`
}

type BlobACLPolicy struct {
	Mode      string   `json:"mode"`
	Allowlist []string `json:"allowlist,omitempty"`
	Enforced  bool     `json:"enforced"`
}

type BlobNodePresetConfig struct {
	Preset                  string `json:"preset"`
	StorageProtection       string `json:"storage_protection_mode"`
	Retention               string `json:"content_retention_mode"`
	ImageQuotaMB            int    `json:"image_quota_mb"`
	FileQuotaMB             int    `json:"file_quota_mb"`
	ImageMaxItemSizeMB      int    `json:"image_max_item_size_mb"`
	FileMaxItemSizeMB       int    `json:"file_max_item_size_mb"`
	ReplicationMode         string `json:"replication_mode"`
	AnnounceEnabled         bool   `json:"announce_enabled"`
	FetchEnabled            bool   `json:"fetch_enabled"`
	RolloutPercent          int    `json:"rollout_percent"`
	ServeBandwidthKBps      int    `json:"serve_bandwidth_kbps"`
	FetchBandwidthKBps      int    `json:"fetch_bandwidth_kbps"`
	HighWatermarkPercent    int    `json:"high_watermark_percent"`
	FullCapPercent          int    `json:"full_cap_percent"`
	AggressiveTargetPercent int    `json:"aggressive_target_percent"`
}

type NodeBindingRecord struct {
	IdentityID       string    `json:"identity_id"`
	NodeID           string    `json:"node_id"`
	NodePublicKey    string    `json:"node_public_key"`
	AccountSignature string    `json:"account_signature"`
	NodeSignature    string    `json:"node_signature"`
	BoundAt          time.Time `json:"bound_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type NodeBindingLinkCode struct {
	LinkCode   string    `json:"link_code"`
	Challenge  string    `json:"challenge"`
	ExpiresAt  time.Time `json:"expires_at"`
	IdentityID string    `json:"identity_id"`
}

func ClassifyAttachmentMime(mimeType string) AttachmentClass {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(mimeType, "image/") {
		return AttachmentClassImage
	}
	return AttachmentClassFile
}

type MessageRequest struct {
	SenderID        string    `json:"sender_id"`
	FirstMessageAt  time.Time `json:"first_message_at"`
	LastMessageAt   time.Time `json:"last_message_at"`
	MessageCount    int       `json:"message_count"`
	LastMessageID   string    `json:"last_message_id"`
	LastContentType string    `json:"last_content_type"`
	LastPreview     string    `json:"last_preview"`
}

type MessageRequestThread struct {
	Request  MessageRequest `json:"request"`
	Messages []Message      `json:"messages"`
}

type BlockSenderResult struct {
	Blocked        []string `json:"blocked"`
	RequestRemoved bool     `json:"request_removed"`
	ContactExists  bool     `json:"contact_exists"`
}
