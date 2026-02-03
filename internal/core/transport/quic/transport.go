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
	peerPubs           map[string]ed25519.PublicKey
	mu                 sync.RWMutex
	peerAddrs          map[string]string
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
	return &Server{
		cfg:       cfg,
		keys:      keys,
		peerID:    peerID,
		stopCh:    make(chan struct{}),
		peerAddrs: make(map[string]string),
		peerPubs:  make(map[string]ed25519.PublicKey),
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
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn quicgo.Connection) {
	defer s.wg.Done()
	defer func() {
		_ = conn.CloseWithError(0, "")
	}()
	connected := false
	peerID := ""
	defer func() {
		if connected && s.onPeerDisconnected != nil {
			s.onPeerDisconnected(peerID)
		}
	}()

	cs := conn.ConnectionState().TLS
	if len(cs.PeerCertificates) == 0 {
		return
	}
	peerIDFromCert, err := PeerIDFromCert(cs.PeerCertificates[0])
	if err != nil || peerIDFromCert == "" {
		return
	}
	var peerPub ed25519.PublicKey
	if pk, ok := cs.PeerCertificates[0].PublicKey.(ed25519.PublicKey); ok {
		peerPub = pk
	}

	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		return
	}
	defer func() {
		_ = stream.Close()
	}()

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
		return
	}
	if err := writeFrame(stream, localBytes); err != nil {
		return
	}
	remoteBytes, err := readFrame(stream, s.cfg.Limits.MaxMsgBytes)
	if err != nil {
		return
	}
	remoteHello, err := DecodeHello(remoteBytes)
	if err != nil {
		return
	}
	if s.onHello != nil {
		s.onHello(remoteHello.PeerID, remoteHello.TSMs, remoteHello.CapabilitiesDigest)
	}
	if err := ValidateHello(timeutil.NowUnixMs(), remoteHello); err != nil {
		return
	}
	if remoteHello.PeerID != peerIDFromCert {
		return
	}
	peerID = remoteHello.PeerID
	if s.onPeerConnected != nil {
		s.onPeerConnected(peerID)
		connected = true
	}
	s.mu.Lock()
	s.peerAddrs[remoteHello.PeerID] = conn.RemoteAddr().String()
	if len(peerPub) > 0 {
		s.peerPubs[remoteHello.PeerID] = peerPub
	}
	s.mu.Unlock()

	for {
		data, err := readFrame(stream, s.cfg.Limits.MaxMsgBytes)
		if err != nil {
			return
		}
		if s.onEnvelope != nil {
			resps, err := s.onEnvelope(remoteHello.PeerID, data)
			if err == nil && len(resps) > 0 {
				for _, resp := range resps {
					if len(resp) == 0 {
						continue
					}
					if err := writeFrame(stream, resp); err != nil {
						return
					}
				}
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
