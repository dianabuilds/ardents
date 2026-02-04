package quic

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"time"

	quicgo "github.com/quic-go/quic-go"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/conv"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/netaddr"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

var (
	ErrPeerIDMismatch = errors.New("ERR_PEER_ID_MISMATCH")
)

type Dialer struct {
	cfg             config.Config
	keys            KeyMaterial
	peerID          string
	onHello         func(peerID string, remoteTSMs int64, capabilitiesDigest []byte)
	onHint          func(peerID string, routerInfoBytes []byte)
	capDigest       []byte
	hintMu          sync.RWMutex
	routerInfoHint  []byte
	routerInfoHints [][]byte
	mu              sync.RWMutex
	peerPubs        map[string]ed25519.PublicKey
	outboundSem     chan struct{}
}

func NewDialer(cfg config.Config) (*Dialer, error) {
	keys, err := LoadOrCreateKeyMaterial("")
	if err != nil {
		return nil, err
	}
	peerID, err := ids.NewPeerID(keys.PublicKey)
	if err != nil {
		return nil, err
	}
	var outboundSem chan struct{}
	if cfg.Limits.MaxOutboundConns > 0 {
		outboundSem = make(chan struct{}, conv.ClampUint64ToInt(cfg.Limits.MaxOutboundConns))
	}
	return &Dialer{cfg: cfg, keys: keys, peerID: peerID, peerPubs: make(map[string]ed25519.PublicKey), outboundSem: outboundSem}, nil
}

func (d *Dialer) SetHelloObserver(fn func(peerID string, remoteTSMs int64)) {
	d.onHello = func(peerID string, remoteTSMs int64, capabilitiesDigest []byte) {
		fn(peerID, remoteTSMs)
	}
}

func (d *Dialer) SetHelloObserverWithDigest(fn func(peerID string, remoteTSMs int64, capabilitiesDigest []byte)) {
	d.onHello = fn
}

func (d *Dialer) SetCapabilitiesDigest(digest []byte) {
	d.capDigest = digest
}

// SetHandshakeHintObserver observes router_info hints received in hello messages.
// The hint MUST be validated by the receiver (signature/TTL) before use.
func (d *Dialer) SetHandshakeHintObserver(fn func(peerID string, routerInfoBytes []byte)) {
	d.onHint = fn
}

// SetRouterInfoHint configures router_info bytes that this dialer will include in its hello.
// Caller is responsible for providing canonical CBOR bytes of router.info.v1 (including signature).
func (d *Dialer) SetRouterInfoHint(routerInfoBytes []byte) {
	d.hintMu.Lock()
	d.routerInfoHint = append([]byte(nil), routerInfoBytes...)
	// Keep legacy single-hint field and list in sync (list always contains at least self hint if set).
	if len(routerInfoBytes) == 0 {
		d.routerInfoHints = nil
	} else {
		d.routerInfoHints = [][]byte{append([]byte(nil), routerInfoBytes...)}
	}
	d.hintMu.Unlock()
}

// SetRouterInfoHints configures router_infos bytes that this dialer will include in its hello.
// Caller is responsible for providing canonical CBOR bytes of router.info.v1 records (including signatures).
func (d *Dialer) SetRouterInfoHints(routerInfos [][]byte) {
	d.hintMu.Lock()
	d.routerInfoHints = make([][]byte, 0, len(routerInfos))
	for _, b := range routerInfos {
		if len(b) == 0 {
			continue
		}
		d.routerInfoHints = append(d.routerInfoHints, append([]byte(nil), b...))
	}
	// Preserve legacy single field as "self" hint if present.
	if len(d.routerInfoHints) > 0 {
		d.routerInfoHint = append([]byte(nil), d.routerInfoHints[0]...)
	} else {
		d.routerInfoHint = nil
	}
	d.hintMu.Unlock()
}

func (d *Dialer) PeerPublicKey(peerID string) (ed25519.PublicKey, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	pub, ok := d.peerPubs[peerID]
	return pub, ok
}

func (d *Dialer) DialAndHandshake(ctx context.Context, addr string, expectedPeerID string) error {
	if err := d.acquireOutbound(); err != nil {
		return err
	}
	defer d.releaseOutbound()
	conn, stream, err := d.dialAndHandshakeStream(ctx, addr, expectedPeerID)
	if err != nil {
		return err
	}
	_ = stream.Close()
	return conn.CloseWithError(0, "")
}

func (d *Dialer) DialWithRetry(ctx context.Context, addr string, expectedPeerID string) error {
	base := 250 * time.Millisecond
	maxWait := 30 * time.Second
	factor := 2.0
	attempt := 0
	for {
		err := d.DialAndHandshake(ctx, addr, expectedPeerID)
		if err == nil {
			return nil
		}
		attempt++
		wait := time.Duration(float64(base) * powFloat(factor, attempt-1))
		if wait > maxWait {
			wait = maxWait
		}
		jitter := time.Duration(float64(wait) * 0.2)
		if jitter > 0 {
			wait = wait - jitter/2
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

func powFloat(a float64, n int) float64 {
	if n <= 0 {
		return 1
	}
	r := 1.0
	for i := 0; i < n; i++ {
		r *= a
	}
	return r
}

func (d *Dialer) SendEnvelope(ctx context.Context, addr string, expectedPeerID string, envelopeBytes []byte, maxBytes uint64) ([]byte, error) {
	if err := d.acquireOutbound(); err != nil {
		return nil, err
	}
	defer d.releaseOutbound()
	conn, stream, err := d.dialAndHandshakeStream(ctx, addr, expectedPeerID)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = stream.Close()
		_ = conn.CloseWithError(0, "")
	}()
	if err := writeFrame(stream, envelopeBytes); err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = stream.SetReadDeadline(deadline)
	}
	ackBytes, err := readFrame(stream, maxBytes)
	_ = stream.SetReadDeadline(time.Time{})
	if err != nil {
		return nil, err
	}
	return ackBytes, nil
}

func (d *Dialer) SendEnvelopeWithReply(ctx context.Context, addr string, expectedPeerID string, envelopeBytes []byte, maxBytes uint64, shouldRead func([]byte) bool, replyTimeout time.Duration) ([]byte, []byte, error) {
	// Reuse the general "read until terminal reply" helper for the simple 1-reply case.
	return d.SendEnvelopeWithReplyUntil(ctx, addr, expectedPeerID, envelopeBytes, maxBytes, shouldRead, replyTimeout, nil)
}

// SendEnvelopeWithReplyUntil is like SendEnvelopeWithReply, but keeps reading reply frames
// while shouldContinue returns true. This is needed for protocols where the peer may
// send intermediate frames (e.g. accept/progress) before the terminal reply.
func (d *Dialer) SendEnvelopeWithReplyUntil(ctx context.Context, addr string, expectedPeerID string, envelopeBytes []byte, maxBytes uint64, shouldRead func([]byte) bool, replyTimeout time.Duration, shouldContinue func([]byte) bool) ([]byte, []byte, error) {
	ackBytes, _, stream, cleanup, err := d.sendEnvelopeAndReadAck(ctx, addr, expectedPeerID, envelopeBytes, maxBytes)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()
	if shouldRead != nil && !shouldRead(ackBytes) {
		return ackBytes, nil, nil
	}
	if replyTimeout > 0 {
		_ = stream.SetReadDeadline(time.Now().Add(replyTimeout))
	}
	const maxReplies = 128
	for i := 0; i < maxReplies; i++ {
		respBytes, err := readFrame(stream, maxBytes)
		if err != nil {
			if replyTimeout > 0 {
				_ = stream.SetReadDeadline(time.Time{})
			}
			return ackBytes, nil, err
		}
		if shouldContinue != nil && shouldContinue(respBytes) {
			continue
		}
		if replyTimeout > 0 {
			_ = stream.SetReadDeadline(time.Time{})
		}
		return ackBytes, respBytes, nil
	}
	if replyTimeout > 0 {
		_ = stream.SetReadDeadline(time.Time{})
	}
	return ackBytes, nil, errors.New("ERR_QUIC_TOO_MANY_REPLIES")
}

func (d *Dialer) sendEnvelopeAndReadAck(ctx context.Context, addr string, expectedPeerID string, envelopeBytes []byte, maxBytes uint64) ([]byte, *quicgo.Conn, *quicgo.Stream, func(), error) {
	if err := d.acquireOutbound(); err != nil {
		return nil, nil, nil, func() {}, err
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		d.releaseOutbound()
	}
	conn, stream, err := d.dialAndHandshakeStream(ctx, addr, expectedPeerID)
	if err != nil {
		release()
		return nil, nil, nil, func() {}, err
	}
	cleanup := func() {
		_ = stream.Close()
		_ = conn.CloseWithError(0, "")
		release()
	}
	if err := writeFrame(stream, envelopeBytes); err != nil {
		cleanup()
		return nil, nil, nil, func() {}, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = stream.SetReadDeadline(deadline)
	}
	ackBytes, err := readFrame(stream, maxBytes)
	_ = stream.SetReadDeadline(time.Time{})
	if err != nil {
		cleanup()
		return nil, nil, nil, func() {}, err
	}
	return ackBytes, conn, stream, cleanup, nil
}

func (d *Dialer) dialAndHandshakeStream(ctx context.Context, addr string, expectedPeerID string) (*quicgo.Conn, *quicgo.Stream, error) {
	addr = netaddr.StripQUICScheme(addr)
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, nil, ErrAddrInvalid
	}
	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{d.keys.TLSCert},
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // #nosec G402 -- peer identity verified via certificate-derived peer ID.
	}
	quicConf := &quicgo.Config{
		HandshakeIdleTimeout: 10 * time.Second,
		MaxIdleTimeout:       30 * time.Second,
		KeepAlivePeriod:      10 * time.Second,
	}
	conn, err := quicgo.DialAddr(ctx, addr, tlsConf, quicConf)
	if err != nil {
		return nil, nil, err
	}
	peerIDFromCert, err := d.storePeerCert(conn, expectedPeerID)
	if err != nil {
		_ = conn.CloseWithError(0, "")
		return nil, nil, err
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(0, "")
		return nil, nil, err
	}
	if err := d.exchangeHello(stream, peerIDFromCert, expectedPeerID); err != nil {
		_ = stream.Close()
		_ = conn.CloseWithError(0, "")
		return nil, nil, err
	}
	return conn, stream, nil
}

func (d *Dialer) acquireOutbound() error {
	if d.outboundSem == nil {
		return nil
	}
	select {
	case d.outboundSem <- struct{}{}:
		return nil
	default:
		return ErrMaxOutboundConns
	}
}

func (d *Dialer) releaseOutbound() {
	if d.outboundSem == nil {
		return
	}
	select {
	case <-d.outboundSem:
	default:
	}
}

func (d *Dialer) storePeerCert(conn *quicgo.Conn, expectedPeerID string) (string, error) {
	cs := conn.ConnectionState().TLS
	if len(cs.PeerCertificates) == 0 {
		return "", ErrPeerCertInvalid
	}
	peerID, err := PeerIDFromCert(cs.PeerCertificates[0])
	if err != nil {
		return "", err
	}
	if pk, ok := cs.PeerCertificates[0].PublicKey.(ed25519.PublicKey); ok {
		d.mu.Lock()
		d.peerPubs[peerID] = pk
		d.mu.Unlock()
	}
	if expectedPeerID != "" && peerID != expectedPeerID {
		return "", ErrPeerIDMismatch
	}
	return peerID, nil
}

func (d *Dialer) exchangeHello(stream *quicgo.Stream, peerIDFromCert string, expectedPeerID string) error {
	d.hintMu.RLock()
	routerInfoHint := append([]byte(nil), d.routerInfoHint...)
	routerInfoHints := make([][]byte, 0, len(d.routerInfoHints))
	for _, b := range d.routerInfoHints {
		if len(b) == 0 {
			continue
		}
		routerInfoHints = append(routerInfoHints, append([]byte(nil), b...))
	}
	d.hintMu.RUnlock()
	localHello := Hello{
		V:                  HelloVersion,
		PeerID:             d.peerID,
		TSMs:               timeutil.NowUnixMs(),
		Nonce:              make([]byte, 16),
		PowDifficulty:      d.cfg.Pow.DefaultDifficulty,
		MaxMsgBytes:        d.cfg.Limits.MaxMsgBytes,
		CapabilitiesDigest: d.capDigest,
		RouterInfo:         routerInfoHint,
		RouterInfos:        routerInfoHints,
	}
	localBytes, err := EncodeHello(localHello)
	if err != nil {
		return err
	}
	if err := writeFrame(stream, localBytes); err != nil {
		return err
	}
	remoteBytes, err := readFrame(stream, d.cfg.Limits.MaxMsgBytes)
	if err != nil {
		return err
	}
	remoteHello, err := DecodeHello(remoteBytes)
	if err != nil {
		return err
	}
	if d.onHello != nil {
		d.onHello(remoteHello.PeerID, remoteHello.TSMs, remoteHello.CapabilitiesDigest)
	}
	if d.onHint != nil && len(remoteHello.RouterInfo) > 0 {
		d.onHint(remoteHello.PeerID, remoteHello.RouterInfo)
	}
	if d.onHint != nil && len(remoteHello.RouterInfos) > 0 {
		const maxHintsPerHello = 16
		seen := 0
		for _, b := range remoteHello.RouterInfos {
			if seen >= maxHintsPerHello {
				break
			}
			if len(b) == 0 {
				continue
			}
			// Avoid duplicating the legacy single-hint field when both are populated.
			if len(remoteHello.RouterInfo) > 0 && bytes.Equal(b, remoteHello.RouterInfo) {
				continue
			}
			d.onHint(remoteHello.PeerID, b)
			seen++
		}
	}
	if err := ValidateHello(timeutil.NowUnixMs(), remoteHello); err != nil {
		return err
	}
	if remoteHello.PeerID != peerIDFromCert {
		return ErrPeerIDMismatch
	}
	if expectedPeerID != "" && remoteHello.PeerID != expectedPeerID {
		return ErrPeerIDMismatch
	}
	return nil
}
