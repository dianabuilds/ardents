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
	Status                   string    `json:"status"`
	PeerCount                int       `json:"peer_count"`
	PeerTarget               int       `json:"peer_target,omitempty"`
	HealthSummary            string    `json:"health_summary,omitempty"`
	ActionHint               string    `json:"action_hint,omitempty"`
	ProfileID                string    `json:"profile_id,omitempty"`
	RelayEnabled             bool      `json:"relay_enabled,omitempty"`
	PublicDiscoveryEnabled   bool      `json:"public_discovery_enabled,omitempty"`
	PublicServingEnabled     bool      `json:"public_serving_enabled,omitempty"`
	PublicStoreEnabled       bool      `json:"public_store_enabled,omitempty"`
	PersonalStoreEnabled     bool      `json:"personal_store_enabled,omitempty"`
	LastSync                 time.Time `json:"last_sync"`
	BootstrapSource          string    `json:"bootstrap_source,omitempty"`
	BootstrapManifestVersion int       `json:"bootstrap_manifest_version,omitempty"`
	BootstrapManifestKeyID   string    `json:"bootstrap_manifest_key_id,omitempty"`
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
	Preset                     string `json:"preset"`
	ProfileID                  string `json:"profile_id,omitempty"`
	StorageProtection          string `json:"storage_protection_mode"`
	Retention                  string `json:"content_retention_mode"`
	ImageQuotaMB               int    `json:"image_quota_mb"`
	FileQuotaMB                int    `json:"file_quota_mb"`
	ImageMaxItemSizeMB         int    `json:"image_max_item_size_mb"`
	FileMaxItemSizeMB          int    `json:"file_max_item_size_mb"`
	ReplicationMode            string `json:"replication_mode"`
	RelayEnabled               bool   `json:"relay_enabled,omitempty"`
	PublicDiscoveryEnabled     bool   `json:"public_discovery_enabled,omitempty"`
	PublicServingEnabled       bool   `json:"public_serving_enabled,omitempty"`
	PublicStoreEnabled         bool   `json:"public_store_enabled,omitempty"`
	PersonalStoreEnabled       bool   `json:"personal_store_enabled,omitempty"`
	AnnounceEnabled            bool   `json:"announce_enabled"`
	FetchEnabled               bool   `json:"fetch_enabled"`
	RolloutPercent             int    `json:"rollout_percent"`
	ServeBandwidthKBps         int    `json:"serve_bandwidth_kbps"`
	ServeBandwidthSoftKBps     int    `json:"serve_bandwidth_soft_kbps,omitempty"`
	ServeBandwidthHardKBps     int    `json:"serve_bandwidth_hard_kbps,omitempty"`
	ServeMaxConcurrent         int    `json:"serve_max_concurrent,omitempty"`
	ServeRequestsPerMinPerPeer int    `json:"serve_requests_per_min_per_peer,omitempty"`
	PublicEphemeralCacheMaxMB  int    `json:"public_ephemeral_cache_max_mb,omitempty"`
	PublicEphemeralCacheTTLMin int    `json:"public_ephemeral_cache_ttl_min,omitempty"`
	FetchBandwidthKBps         int    `json:"fetch_bandwidth_kbps"`
	HighWatermarkPercent       int    `json:"high_watermark_percent"`
	FullCapPercent             int    `json:"full_cap_percent"`
	AggressiveTargetPercent    int    `json:"aggressive_target_percent"`
}

type NodePersonalPolicy struct {
	StoreEnabled bool `json:"store_enabled"`
	TTLDays      int  `json:"ttl_days,omitempty"`
	QuotaMB      int  `json:"quota_mb,omitempty"`
	PinEnabled   bool `json:"pin_enabled,omitempty"`
}

type NodePublicPolicy struct {
	RelayEnabled     bool `json:"relay_enabled"`
	DiscoveryEnabled bool `json:"discovery_enabled"`
	ServingEnabled   bool `json:"serving_enabled"`
	StoreEnabled     bool `json:"store_enabled"`
	TTLDays          int  `json:"ttl_days,omitempty"`
	QuotaMB          int  `json:"quota_mb,omitempty"`
}

type NodePolicies struct {
	ProfileSchemaVersion int                `json:"profile_schema_version,omitempty"`
	Personal             NodePersonalPolicy `json:"personal_policy"`
	Public               NodePublicPolicy   `json:"public_policy"`
}

type NodePersonalPolicyPatch struct {
	StoreEnabled *bool `json:"store_enabled,omitempty"`
	TTLDays      *int  `json:"ttl_days,omitempty"`
	QuotaMB      *int  `json:"quota_mb,omitempty"`
	PinEnabled   *bool `json:"pin_enabled,omitempty"`
}

type NodePublicPolicyPatch struct {
	RelayEnabled     *bool `json:"relay_enabled,omitempty"`
	DiscoveryEnabled *bool `json:"discovery_enabled,omitempty"`
	ServingEnabled   *bool `json:"serving_enabled,omitempty"`
	StoreEnabled     *bool `json:"store_enabled,omitempty"`
	TTLDays          *int  `json:"ttl_days,omitempty"`
	QuotaMB          *int  `json:"quota_mb,omitempty"`
}

type NodePoliciesPatch struct {
	Personal *NodePersonalPolicyPatch `json:"personal_policy,omitempty"`
	Public   *NodePublicPolicyPatch   `json:"public_policy,omitempty"`
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

type DiagnosticsExportPackage struct {
	SchemaVersion int                          `json:"schema_version"`
	ExportedAt    time.Time                    `json:"exported_at"`
	AppVersion    string                       `json:"app_version"`
	NodeVersion   string                       `json:"node_version"`
	ProfileConfig DiagnosticsProfileConfig     `json:"profile_config"`
	Metrics       DiagnosticsAggregatedMetrics `json:"metrics"`
	Events        []DiagnosticsEvent           `json:"events"`
}

type DiagnosticsProfileConfig struct {
	ProfileID                  string `json:"profile_id,omitempty"`
	Preset                     string `json:"preset,omitempty"`
	RelayEnabled               bool   `json:"relay_enabled"`
	PublicDiscoveryEnabled     bool   `json:"public_discovery_enabled"`
	PublicServingEnabled       bool   `json:"public_serving_enabled"`
	PublicStoreEnabled         bool   `json:"public_store_enabled"`
	PersonalStoreEnabled       bool   `json:"personal_store_enabled"`
	ServeBandwidthSoftKBps     int    `json:"serve_bandwidth_soft_kbps,omitempty"`
	ServeBandwidthHardKBps     int    `json:"serve_bandwidth_hard_kbps,omitempty"`
	ServeMaxConcurrent         int    `json:"serve_max_concurrent,omitempty"`
	ServeRequestsPerMinPerPeer int    `json:"serve_requests_per_min_per_peer,omitempty"`
	PublicEphemeralCacheMaxMB  int    `json:"public_ephemeral_cache_max_mb,omitempty"`
	PublicEphemeralCacheTTLMin int    `json:"public_ephemeral_cache_ttl_min,omitempty"`
	FetchBandwidthKBps         int    `json:"fetch_bandwidth_kbps,omitempty"`
}

type DiagnosticsAggregatedMetrics struct {
	PeerCount           int              `json:"peer_count"`
	PendingQueueSize    int              `json:"pending_queue_size"`
	RetryAttemptsTotal  int              `json:"retry_attempts_total"`
	NotificationBacklog int              `json:"notification_backlog"`
	ErrorCounters       map[string]int   `json:"error_counters,omitempty"`
	BlobFetchStats      BlobFetchMetric  `json:"blob_fetch_stats,omitempty"`
	DiskUsageByClass    map[string]int64 `json:"disk_usage_by_class,omitempty"`
}

type DiagnosticsEvent struct {
	Level      string    `json:"level"`
	OccurredAt time.Time `json:"occurred_at"`
	Operation  string    `json:"operation,omitempty"`
	Message    string    `json:"message"`
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
