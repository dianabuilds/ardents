package netdb

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/codec"
)

const (
	MaxRecordBytes        = 64 * 1024
	MaxLeasesPerLeaseSet  = 16
	MaxAddrsPerRouter     = 8
	DefaultRecordMaxTTLMs = int64(3600_000)
	DefaultK              = 20
)

var (
	ErrBadRecord      = errors.New("ERR_NETDB_BAD_RECORD")
	ErrSigInvalid     = errors.New("ERR_NETDB_SIG_INVALID")
	ErrExpired        = errors.New("ERR_NETDB_EXPIRED")
	ErrTooLarge       = errors.New("ERR_NETDB_TOO_LARGE")
	ErrNotAuthorized  = errors.New("ERR_NETDB_NOT_AUTHORIZED")
	ErrPowRequired    = errors.New("ERR_NETDB_POW_REQUIRED")
	ErrPowInvalid     = errors.New("ERR_NETDB_POW_INVALID")
	ErrRecordRejected = errors.New("ERR_NETDB_REJECTED")
)

var _ = []error{
	ErrNotAuthorized,
	ErrPowRequired,
	ErrPowInvalid,
	ErrRecordRejected,
}

type DB struct {
	mu       sync.Mutex
	maxTTLms int64
	k        int

	routers map[string]Record
	leases  map[string]Record
	heads   map[string]Record
	byKey   map[string]Record
}

type Record struct {
	Type          string
	Address       string
	PublishedAtMs int64
	ExpiresAtMs   int64
	Bytes         []byte
}

func New(maxTTLms int64, k int) *DB {
	if maxTTLms <= 0 {
		maxTTLms = DefaultRecordMaxTTLMs
	}
	if k <= 0 {
		k = DefaultK
	}
	return &DB{
		maxTTLms: maxTTLms,
		k:        k,
		routers:  map[string]Record{},
		leases:   map[string]Record{},
		heads:    map[string]Record{},
		byKey:    map[string]Record{},
	}
}

func (db *DB) UpdateParams(maxTTLms int64, k int) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if maxTTLms > 0 {
		db.maxTTLms = maxTTLms
	}
	if k > 0 {
		db.k = k
	}
}

func (db *DB) K() int {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.k
}

func (db *DB) Store(value []byte, nowMs int64) (status string, code string) {
	if len(value) == 0 {
		return "REJECTED", ErrBadRecord.Error()
	}
	if len(value) > MaxRecordBytes {
		return "REJECTED", ErrTooLarge.Error()
	}
	rec, err := db.parseRecord(value, nowMs)
	if err != nil {
		return "REJECTED", err.Error()
	}
	key := dhtKey(rec.Type, rec.Address)
	keyHex := string(key[:])

	db.mu.Lock()
	defer db.mu.Unlock()
	if existing, ok := db.byKey[keyHex]; ok {
		if !isNewer(rec, existing) {
			return "OK", ""
		}
	}
	db.byKey[keyHex] = rec
	switch rec.Type {
	case TypeRouterInfo:
		db.routers[rec.Address] = rec
	case TypeLeaseSet:
		db.leases[rec.Address] = rec
	case TypeServiceHead:
		db.heads[rec.Address] = rec
	}
	return "OK", ""
}

func (db *DB) FindValue(key []byte) ([]byte, bool) {
	if len(key) != 32 {
		return nil, false
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	rec, ok := db.byKey[string(key)]
	if !ok {
		return nil, false
	}
	if rec.ExpiresAtMs <= time.Now().UTC().UnixNano()/int64(time.Millisecond) {
		return nil, false
	}
	return rec.Bytes, true
}

func (db *DB) FindClosestNodes(key []byte, limit int) []string {
	db.mu.Lock()
	defer db.mu.Unlock()
	if limit <= 0 {
		limit = db.k
	}
	if limit <= 0 {
		limit = DefaultK
	}
	type item struct {
		peerID   string
		distance [32]byte
	}
	items := make([]item, 0, len(db.routers))
	for peerID := range db.routers {
		rk := dhtKey(TypeRouterInfo, peerID)
		items = append(items, item{
			peerID:   peerID,
			distance: xorDistance(rk, key),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return bytes.Compare(items[i].distance[:], items[j].distance[:]) < 0
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.peerID)
	}
	return out
}

func (db *DB) RoutersSnapshot(nowMs int64) []RouterInfo {
	db.mu.Lock()
	defer db.mu.Unlock()
	out := make([]RouterInfo, 0, len(db.routers))
	for _, rec := range db.routers {
		if rec.ExpiresAtMs <= nowMs {
			continue
		}
		var r RouterInfo
		if err := codec.Unmarshal(rec.Bytes, &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (db *DB) Router(peerID string, nowMs int64) (RouterInfo, bool) {
	if peerID == "" {
		return RouterInfo{}, false
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	rec, ok := db.routers[peerID]
	return decodeRecord[RouterInfo](rec, ok, nowMs)
}

func (db *DB) LeaseSet(serviceID string, nowMs int64) (LeaseSet, bool) {
	if serviceID == "" {
		return LeaseSet{}, false
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	rec, ok := db.leases[serviceID]
	return decodeRecord[LeaseSet](rec, ok, nowMs)
}

func (db *DB) ServiceHead(serviceID string, nowMs int64) (ServiceHead, bool) {
	if serviceID == "" {
		return ServiceHead{}, false
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	rec, ok := db.heads[serviceID]
	return decodeRecord[ServiceHead](rec, ok, nowMs)
}

func (db *DB) ServiceHeadsSnapshot(nowMs int64) []ServiceHead {
	db.mu.Lock()
	defer db.mu.Unlock()
	out := make([]ServiceHead, 0, len(db.heads))
	for _, rec := range db.heads {
		h, ok := decodeRecord[ServiceHead](rec, true, nowMs)
		if ok {
			out = append(out, h)
		}
	}
	return out
}

func decodeRecord[T any](rec Record, ok bool, nowMs int64) (T, bool) {
	var out T
	if !ok || rec.ExpiresAtMs <= nowMs {
		return out, false
	}
	if err := codec.Unmarshal(rec.Bytes, &out); err != nil {
		return out, false
	}
	return out, true
}

func dhtKey(typ, address string) [32]byte {
	b := []byte(typ + "\x00" + address)
	return sha256.Sum256(b)
}

func xorDistance(a [32]byte, b []byte) [32]byte {
	var out [32]byte
	if len(b) < 32 {
		return out
	}
	for i := 0; i < 32; i++ {
		out[i] = a[i] ^ b[i]
	}
	return out
}

func isNewer(a Record, b Record) bool {
	if a.PublishedAtMs > b.PublishedAtMs {
		return true
	}
	if a.PublishedAtMs == b.PublishedAtMs && a.ExpiresAtMs > b.ExpiresAtMs {
		return true
	}
	return false
}

const (
	TypeRouterInfo  = "router.info.v1"
	TypeLeaseSet    = "service.lease_set.v1"
	TypeServiceHead = "service.head.v1"
)

type RouterInfo struct {
	V             uint64     `cbor:"v"`
	PeerID        string     `cbor:"peer_id"`
	TransportPub  []byte     `cbor:"transport_pub"`
	OnionPub      []byte     `cbor:"onion_pub"`
	Addrs         []string   `cbor:"addrs"`
	Caps          RouterCaps `cbor:"caps"`
	PublishedAtMs int64      `cbor:"published_at_ms"`
	ExpiresAtMs   int64      `cbor:"expires_at_ms"`
	Sig           []byte     `cbor:"sig"`
}

type RouterCaps struct {
	Relay bool `cbor:"relay"`
	NetDB bool `cbor:"netdb"`
}

type LeaseSet struct {
	V               uint64  `cbor:"v"`
	ServiceID       string  `cbor:"service_id"`
	OwnerIdentityID string  `cbor:"owner_identity_id"`
	ServiceName     string  `cbor:"service_name"`
	EncPub          []byte  `cbor:"enc_pub"`
	Leases          []Lease `cbor:"leases"`
	PublishedAtMs   int64   `cbor:"published_at_ms"`
	ExpiresAtMs     int64   `cbor:"expires_at_ms"`
	Sig             []byte  `cbor:"sig"`
}

type Lease struct {
	GatewayPeerID string `cbor:"gateway_peer_id"`
	TunnelID      []byte `cbor:"tunnel_id"`
	ExpiresAtMs   int64  `cbor:"expires_at_ms"`
}

type ServiceHead struct {
	V               uint64 `cbor:"v"`
	ServiceID       string `cbor:"service_id"`
	OwnerIdentityID string `cbor:"owner_identity_id"`
	ServiceName     string `cbor:"service_name"`
	DescriptorCID   string `cbor:"descriptor_cid"`
	PublishedAtMs   int64  `cbor:"published_at_ms"`
	ExpiresAtMs     int64  `cbor:"expires_at_ms"`
	Sig             []byte `cbor:"sig"`
}

func (db *DB) parseRecord(value []byte, nowMs int64) (Record, error) {
	if r, ok := tryRouterInfo(value); ok {
		if err := db.validateRouterInfo(r, nowMs); err != nil {
			return Record{}, err
		}
		return Record{
			Type:          TypeRouterInfo,
			Address:       r.PeerID,
			PublishedAtMs: r.PublishedAtMs,
			ExpiresAtMs:   r.ExpiresAtMs,
			Bytes:         value,
		}, nil
	}
	if s, ok := tryLeaseSet(value); ok {
		if err := db.validateLeaseSet(s, nowMs); err != nil {
			return Record{}, err
		}
		return Record{
			Type:          TypeLeaseSet,
			Address:       s.ServiceID,
			PublishedAtMs: s.PublishedAtMs,
			ExpiresAtMs:   s.ExpiresAtMs,
			Bytes:         value,
		}, nil
	}
	if h, ok := tryServiceHead(value); ok {
		if err := db.validateServiceHead(h, nowMs); err != nil {
			return Record{}, err
		}
		return Record{
			Type:          TypeServiceHead,
			Address:       h.ServiceID,
			PublishedAtMs: h.PublishedAtMs,
			ExpiresAtMs:   h.ExpiresAtMs,
			Bytes:         value,
		}, nil
	}
	return Record{}, ErrBadRecord
}

func tryRouterInfo(b []byte) (RouterInfo, bool) {
	var r RouterInfo
	if err := codec.Unmarshal(b, &r); err != nil {
		return RouterInfo{}, false
	}
	if r.PeerID == "" || len(r.TransportPub) == 0 {
		return RouterInfo{}, false
	}
	return r, true
}

func tryLeaseSet(b []byte) (LeaseSet, bool) {
	var s LeaseSet
	if err := codec.Unmarshal(b, &s); err != nil {
		return LeaseSet{}, false
	}
	if s.ServiceID == "" || s.OwnerIdentityID == "" || s.ServiceName == "" {
		return LeaseSet{}, false
	}
	if len(s.EncPub) == 0 || len(s.Leases) == 0 {
		return LeaseSet{}, false
	}
	return s, true
}

func tryServiceHead(b []byte) (ServiceHead, bool) {
	var h ServiceHead
	if err := codec.Unmarshal(b, &h); err != nil {
		return ServiceHead{}, false
	}
	if h.ServiceID == "" || h.OwnerIdentityID == "" || h.ServiceName == "" || h.DescriptorCID == "" {
		return ServiceHead{}, false
	}
	return h, true
}
