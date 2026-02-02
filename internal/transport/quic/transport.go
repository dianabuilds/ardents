package quic

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	quicgo "github.com/quic-go/quic-go"

	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

type Transport interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Addr() string
}

type Server struct {
	cfg        config.Config
	keys       KeyMaterial
	peerID     string
	listener   *quicgo.Listener
	addr       string
	stopCh     chan struct{}
	wg         sync.WaitGroup
	onEnvelope func(fromPeerID string, data []byte) ([][]byte, error)
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
		cfg:    cfg,
		keys:   keys,
		peerID: peerID,
		stopCh: make(chan struct{}),
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

func (s *Server) SetEnvelopeHandler(fn func(fromPeerID string, data []byte) ([][]byte, error)) {
	s.onEnvelope = fn
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

	cs := conn.ConnectionState().TLS
	if len(cs.PeerCertificates) == 0 {
		return
	}
	peerIDFromCert, err := PeerIDFromCert(cs.PeerCertificates[0])
	if err != nil || peerIDFromCert == "" {
		return
	}

	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		return
	}
	defer func() {
		_ = stream.Close()
	}()

	localHello := Hello{
		V:             HelloVersion,
		PeerID:        s.peerID,
		TSMs:          timeutil.NowUnixMs(),
		Nonce:         make([]byte, 16),
		PowDifficulty: s.cfg.Pow.DefaultDifficulty,
		MaxMsgBytes:   s.cfg.Limits.MaxMsgBytes,
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
	if err := ValidateHello(timeutil.NowUnixMs(), remoteHello); err != nil {
		return
	}
	if remoteHello.PeerID != peerIDFromCert {
		return
	}

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
