package quic

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"net"
	"sync"
	"time"

	quicgo "github.com/quic-go/quic-go"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/conv"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

type Transport interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Addr() string
}

type Server struct {
	cfg                config.Config
	keys               KeyMaterial
	peerID             string
	listener           *quicgo.Listener
	addr               string
	stopCh             chan struct{}
	wg                 sync.WaitGroup
	onEnvelope         func(fromPeerID string, data []byte) ([][]byte, error)
	onHello            func(peerID string, remoteTSMs int64, capabilitiesDigest []byte)
	capDigest          []byte
	onPeerConnected    func(peerID string)
	onPeerDisconnected func(peerID string)
	onHandshakeError   func(peerID string, remoteAddr string, err error)
	onConnError        func(peerID string, remoteAddr string, stage string, err error)
	isPeerBanned       func(peerID string) bool
	peerPubs           map[string]ed25519.PublicKey
	mu                 sync.RWMutex
	peerAddrs          map[string]string
	inboundSem         chan struct{}
	handshakeLimiter   *attemptLimiter
}

func NewServer(cfg config.Config) (*Server, error) {
	keys, err := LoadOrCreateKeyMaterial("")
	if err != nil {
		return nil, err
	}
	peerID, err := ids.NewPeerID(keys.PublicKey)
	if err != nil {
		return nil, err
	}
	var inboundSem chan struct{}
	if cfg.Limits.MaxInboundConns > 0 {
		inboundSem = make(chan struct{}, conv.ClampUint64ToInt(cfg.Limits.MaxInboundConns))
	}
	window := time.Duration(cfg.Limits.HandshakeRateWindowMs) * time.Millisecond
	if window <= 0 {
		window = 10 * time.Second
	}
	handshakeLimit := conv.ClampUint64ToInt(cfg.Limits.HandshakeRateLimit)
	return &Server{
		cfg:              cfg,
		keys:             keys,
		peerID:           peerID,
		stopCh:           make(chan struct{}),
		peerAddrs:        make(map[string]string),
		peerPubs:         make(map[string]ed25519.PublicKey),
		inboundSem:       inboundSem,
		handshakeLimiter: newAttemptLimiter(handshakeLimit, window),
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{s.keys.TLSCert},
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		ClientAuth:   tls.RequireAnyClientCert,
	}
	quicConf := &quicgo.Config{
		HandshakeIdleTimeout: 10 * time.Second,
		MaxIdleTimeout:       30 * time.Second,
		KeepAlivePeriod:      10 * time.Second,
	}
	addr := s.cfg.Listen.QUICAddr
	if addr == "" {
		addr = "0.0.0.0:0"
	}
	l, err := quicgo.ListenAddr(addr, tlsConf, quicConf)
	if err != nil {
		return err
	}
	s.listener = l
	s.addr = l.Addr().String()

	s.wg.Add(1)
	go s.acceptLoop(ctx)
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	close(s.stopCh)
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			return err
		}
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) PeerID() string {
	return s.peerID
}

func (s *Server) PeerAddr(peerID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	addr, ok := s.peerAddrs[peerID]
	return addr, ok
}

func (s *Server) PeerPublicKey(peerID string) (ed25519.PublicKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pub, ok := s.peerPubs[peerID]
	return pub, ok
}

func (s *Server) SetEnvelopeHandler(fn func(fromPeerID string, data []byte) ([][]byte, error)) {
	s.onEnvelope = fn
}

func (s *Server) SetHelloObserver(fn func(peerID string, remoteTSMs int64)) {
	s.onHello = func(peerID string, remoteTSMs int64, capabilitiesDigest []byte) {
		fn(peerID, remoteTSMs)
	}
}

func (s *Server) SetHelloObserverWithDigest(fn func(peerID string, remoteTSMs int64, capabilitiesDigest []byte)) {
	s.onHello = fn
}

func (s *Server) SetCapabilitiesDigest(digest []byte) {
	s.capDigest = digest
}

func (s *Server) SetPeerObserver(connected func(peerID string), disconnected func(peerID string)) {
	s.onPeerConnected = connected
	s.onPeerDisconnected = disconnected
}

func (s *Server) SetHandshakeErrorObserver(fn func(peerID string, remoteAddr string, err error)) {
	s.onHandshakeError = fn
}

func (s *Server) SetConnErrorObserver(fn func(peerID string, remoteAddr string, stage string, err error)) {
	s.onConnError = fn
}

func (s *Server) SetPeerBanChecker(fn func(peerID string) bool) {
	s.isPeerBanned = fn
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept(ctx)
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				continue
			}
		}
		if s.inboundSem != nil {
			select {
			case s.inboundSem <- struct{}{}:
			default:
				if s.onHandshakeError != nil {
					s.onHandshakeError("", conn.RemoteAddr().String(), ErrMaxInboundConns)
				}
				_ = conn.CloseWithError(0, "")
				continue
			}
		}
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn *quicgo.Conn) {
	defer s.wg.Done()
	defer func() {
		_ = conn.CloseWithError(0, "")
		if s.inboundSem != nil {
			select {
			case <-s.inboundSem:
			default:
			}
		}
	}()
	remoteAddr := conn.RemoteAddr().String()
	if s.handshakeLimiter != nil {
		key := normalizeAddrKey(remoteAddr)
		if !s.handshakeLimiter.Allow(key) {
			if s.onHandshakeError != nil {
				s.onHandshakeError("", remoteAddr, ErrHandshakeRateLimited)
			}
			return
		}
	}
	peerID, peerPub, err := s.peerIDFromConn(conn)
	if err != nil {
		if s.onHandshakeError != nil {
			s.onHandshakeError("", remoteAddr, err)
		}
		return
	}
	connected := false
	defer func() {
		if connected && s.onPeerDisconnected != nil {
			s.onPeerDisconnected(peerID)
		}
	}()
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		if s.onHandshakeError != nil {
			s.onHandshakeError(peerID, remoteAddr, err)
		}
		if s.onConnError != nil {
			s.onConnError(peerID, remoteAddr, "accept_stream", err)
		}
		return
	}
	defer func() {
		_ = stream.Close()
	}()
	remoteHello, err := s.exchangeHello(stream)
	if err != nil {
		if s.onHandshakeError != nil {
			s.onHandshakeError(peerID, remoteAddr, err)
		}
		if s.onConnError != nil {
			s.onConnError(peerID, remoteAddr, "exchange_hello", err)
		}
		return
	}
	if remoteHello.PeerID != peerID {
		if s.onHandshakeError != nil {
			s.onHandshakeError(remoteHello.PeerID, remoteAddr, ErrPeerIDMismatch)
		}
		return
	}
	if s.isPeerBanned != nil && s.isPeerBanned(peerID) {
		if s.onHandshakeError != nil {
			s.onHandshakeError(peerID, remoteAddr, ErrPeerBanned)
		}
		return
	}
	if s.onPeerConnected != nil {
		s.onPeerConnected(peerID)
		connected = true
	}
	s.storePeer(peerID, peerPub, conn.RemoteAddr().String())
	s.handleEnvelopeLoop(stream, remoteHello.PeerID, remoteAddr)
}

func (s *Server) peerIDFromConn(conn *quicgo.Conn) (string, ed25519.PublicKey, error) {
	cs := conn.ConnectionState().TLS
	if len(cs.PeerCertificates) == 0 {
		return "", nil, ErrPeerCertInvalid
	}
	peerID, err := PeerIDFromCert(cs.PeerCertificates[0])
	if err != nil || peerID == "" {
		return "", nil, err
	}
	if pk, ok := cs.PeerCertificates[0].PublicKey.(ed25519.PublicKey); ok {
		return peerID, pk, nil
	}
	return peerID, nil, nil
}

func (s *Server) exchangeHello(stream *quicgo.Stream) (Hello, error) {
	localHello := Hello{
		V:                  HelloVersion,
		PeerID:             s.peerID,
		TSMs:               timeutil.NowUnixMs(),
		Nonce:              make([]byte, 16),
		PowDifficulty:      s.cfg.Pow.DefaultDifficulty,
		MaxMsgBytes:        s.cfg.Limits.MaxMsgBytes,
		CapabilitiesDigest: s.capDigest,
	}
	localBytes, err := EncodeHello(localHello)
	if err != nil {
		return Hello{}, err
	}
	if err := writeFrame(stream, localBytes); err != nil {
		return Hello{}, err
	}
	remoteBytes, err := readFrame(stream, s.cfg.Limits.MaxMsgBytes)
	if err != nil {
		return Hello{}, err
	}
	remoteHello, err := DecodeHello(remoteBytes)
	if err != nil {
		return Hello{}, err
	}
	if s.onHello != nil {
		s.onHello(remoteHello.PeerID, remoteHello.TSMs, remoteHello.CapabilitiesDigest)
	}
	if err := ValidateHello(timeutil.NowUnixMs(), remoteHello); err != nil {
		return Hello{}, err
	}
	return remoteHello, nil
}

func (s *Server) storePeer(peerID string, peerPub ed25519.PublicKey, addr string) {
	s.mu.Lock()
	s.peerAddrs[peerID] = addr
	if len(peerPub) > 0 {
		s.peerPubs[peerID] = peerPub
	}
	s.mu.Unlock()
}

func (s *Server) handleEnvelopeLoop(stream *quicgo.Stream, peerID string, remoteAddr string) {
	for {
		data, err := readFrame(stream, s.cfg.Limits.MaxMsgBytes)
		if err != nil {
			if s.onConnError != nil {
				s.onConnError(peerID, remoteAddr, "read_frame", err)
			}
			return
		}
		if s.onEnvelope == nil {
			continue
		}
		resps, err := s.onEnvelope(peerID, data)
		if err != nil || len(resps) == 0 {
			continue
		}
		for _, resp := range resps {
			if len(resp) == 0 {
				continue
			}
			if err := writeFrame(stream, resp); err != nil {
				if s.onConnError != nil {
					s.onConnError(peerID, remoteAddr, "write_frame", err)
				}
				return
			}
		}
	}
}

func ParseQUICAddr(addr string) (string, error) {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "", ErrAddrInvalid
	}
	return "quic://" + addr, nil
}
