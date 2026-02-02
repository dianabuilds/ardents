package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	quicgo "github.com/quic-go/quic-go"

	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

var (
	ErrPeerIDMismatch = errors.New("ERR_PEER_ID_MISMATCH")
)

type Dialer struct {
	cfg    config.Config
	keys   KeyMaterial
	peerID string
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
	return &Dialer{cfg: cfg, keys: keys, peerID: peerID}, nil
}

func (d *Dialer) DialAndHandshake(ctx context.Context, addr string, expectedPeerID string) error {
	addr = stripSchemeLocal(addr)
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return ErrAddrInvalid
	}
	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{d.keys.TLSCert},
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	}
	quicConf := &quicgo.Config{
		HandshakeIdleTimeout: 10 * time.Second,
		MaxIdleTimeout:       30 * time.Second,
		KeepAlivePeriod:      10 * time.Second,
	}
	conn, err := quicgo.DialAddr(ctx, addr, tlsConf, quicConf)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.CloseWithError(0, "")
	}()

	cs := conn.ConnectionState().TLS
	if len(cs.PeerCertificates) == 0 {
		return ErrPeerCertInvalid
	}
	peerID, err := PeerIDFromCert(cs.PeerCertificates[0])
	if err != nil {
		return err
	}
	if expectedPeerID != "" && peerID != expectedPeerID {
		return ErrPeerIDMismatch
	}

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = stream.Close()
	}()

	localHello := Hello{
		V:             HelloVersion,
		PeerID:        d.peerID,
		TSMs:          timeutil.NowUnixMs(),
		Nonce:         make([]byte, 16),
		PowDifficulty: d.cfg.Pow.DefaultDifficulty,
		MaxMsgBytes:   d.cfg.Limits.MaxMsgBytes,
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
	if err := ValidateHello(timeutil.NowUnixMs(), remoteHello); err != nil {
		return err
	}
	if expectedPeerID != "" && remoteHello.PeerID != expectedPeerID {
		return ErrPeerIDMismatch
	}
	return nil
}

func stripSchemeLocal(addr string) string {
	const prefix = "quic://"
	if len(addr) >= len(prefix) && addr[:len(prefix)] == prefix {
		return addr[len(prefix):]
	}
	return addr
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
	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{d.keys.TLSCert},
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	}
	quicConf := &quicgo.Config{
		HandshakeIdleTimeout: 10 * time.Second,
		MaxIdleTimeout:       30 * time.Second,
		KeepAlivePeriod:      10 * time.Second,
	}
	conn, err := quicgo.DialAddr(ctx, addr, tlsConf, quicConf)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = conn.CloseWithError(0, "")
	}()

	cs := conn.ConnectionState().TLS
	if len(cs.PeerCertificates) == 0 {
		return nil, ErrPeerCertInvalid
	}
	peerID, err := PeerIDFromCert(cs.PeerCertificates[0])
	if err != nil {
		return nil, err
	}
	if expectedPeerID != "" && peerID != expectedPeerID {
		return nil, ErrPeerIDMismatch
	}

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = stream.Close()
	}()

	localHello := Hello{
		V:             HelloVersion,
		PeerID:        d.peerID,
		TSMs:          timeutil.NowUnixMs(),
		Nonce:         make([]byte, 16),
		PowDifficulty: d.cfg.Pow.DefaultDifficulty,
		MaxMsgBytes:   d.cfg.Limits.MaxMsgBytes,
	}
	localBytes, err := EncodeHello(localHello)
	if err != nil {
		return nil, err
	}
	if err := writeFrame(stream, localBytes); err != nil {
		return nil, err
	}
	remoteBytes, err := readFrame(stream, d.cfg.Limits.MaxMsgBytes)
	if err != nil {
		return nil, err
	}
	remoteHello, err := DecodeHello(remoteBytes)
	if err != nil {
		return nil, err
	}
	if err := ValidateHello(timeutil.NowUnixMs(), remoteHello); err != nil {
		return nil, err
	}
	if expectedPeerID != "" && remoteHello.PeerID != expectedPeerID {
		return nil, ErrPeerIDMismatch
	}

	if err := writeFrame(stream, envelopeBytes); err != nil {
		return nil, err
	}
	ackBytes, err := readFrame(stream, maxBytes)
	if err != nil {
		return nil, err
	}
	return ackBytes, nil
}
