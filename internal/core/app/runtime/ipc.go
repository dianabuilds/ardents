package runtime

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/perm"
)

const ipcTokenBytes = 32
const ipcTaskTimeout = 30 * time.Second

type ipcServer struct {
	listener net.Listener
	token    string
	handlers map[string]*ipcClient
	mu       sync.Mutex
	rt       *Runtime
}

type ipcClient struct {
	conn  net.Conn
	enc   *json.Encoder
	dec   *json.Decoder
	mu    sync.Mutex
	busy  bool
	alive bool
}

type ipcMessage struct {
	Type            string          `json:"type"`
	Token           string          `json:"token,omitempty"`
	ServiceName     string          `json:"service_name,omitempty"`
	JobTypes        []string        `json:"job_types,omitempty"`
	ServiceID       string          `json:"service_id,omitempty"`
	TaskID          string          `json:"task_id,omitempty"`
	ClientRequestID string          `json:"client_request_id,omitempty"`
	JobType         string          `json:"job_type,omitempty"`
	Input           json.RawMessage `json:"input,omitempty"`
	TSMs            int64           `json:"ts_ms,omitempty"`
	ResultNode      string          `json:"result_node,omitempty"`
	ErrorCode       string          `json:"error_code,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
}

func startIPC(rt *Runtime) (*ipcServer, error) {
	if rt == nil {
		return nil, errors.New("ERR_IPC_DISABLED")
	}
	token, err := rotateIPCToken()
	if err != nil {
		return nil, err
	}
	ln, err := ipcListen()
	if err != nil {
		return nil, err
	}
	s := &ipcServer{
		listener: ln,
		token:    token,
		handlers: make(map[string]*ipcClient),
		rt:       rt,
	}
	go s.acceptLoop()
	rt.log.Event("warn", "ipc", "ipc.enabled", "", "", "")
	return s, nil
}

func (s *ipcServer) stop() {
	if s == nil || s.listener == nil {
		return
	}
	_ = s.listener.Close()
	s.mu.Lock()
	for _, h := range s.handlers {
		_ = h.conn.Close()
	}
	s.handlers = map[string]*ipcClient{}
	s.mu.Unlock()
}

func (s *ipcServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *ipcServer) handleConn(conn net.Conn) {
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var auth ipcMessage
	if err := dec.Decode(&auth); err != nil || auth.Type != "auth" || auth.Token == "" || auth.Token != s.token {
		_ = conn.Close()
		return
	}
	var reg ipcMessage
	if err := dec.Decode(&reg); err != nil || reg.Type != "register" {
		_ = conn.Close()
		return
	}
	if err := ids.ValidateServiceName(reg.ServiceName); err != nil || len(reg.JobTypes) == 0 {
		_ = conn.Close()
		return
	}
	for _, job := range reg.JobTypes {
		if err := ids.ValidateServiceName(job); err != nil {
			_ = conn.Close()
			return
		}
	}
	if reg.JobTypes[0] != reg.ServiceName {
		_ = conn.Close()
		return
	}
	serviceID, err := ids.NewServiceID(s.rt.identity.ID, reg.ServiceName)
	if err != nil {
		_ = conn.Close()
		return
	}
	s.rt.registerLocalService(localServiceInfo{
		ServiceID:   serviceID,
		ServiceName: reg.ServiceName,
	})
	client := &ipcClient{
		conn:  conn,
		enc:   enc,
		dec:   dec,
		alive: true,
	}
	s.mu.Lock()
	for _, job := range reg.JobTypes {
		s.handlers[job] = client
	}
	s.mu.Unlock()
	_ = enc.Encode(ipcMessage{Type: "registered", ServiceID: serviceID})
}

func (s *ipcServer) handler(jobType string) *ipcClient {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handlers[jobType]
}

func (c *ipcClient) requestTask(req tasksRequest) (ipcResult, error) {
	if c == nil || !c.alive {
		return ipcResult{}, errIPCUnavailable
	}
	c.mu.Lock()
	if c.busy {
		c.mu.Unlock()
		return ipcResult{errorCode: "ERR_SERVICE_BUSY"}, nil
	}
	c.busy = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.busy = false
		c.mu.Unlock()
	}()

	msg := ipcMessage{
		Type:            "task",
		TaskID:          req.TaskID,
		ClientRequestID: req.ClientRequestID,
		JobType:         req.JobType,
		Input:           req.Input,
		TSMs:            req.TSMs,
	}
	if err := c.conn.SetDeadline(time.Now().Add(ipcTaskTimeout)); err != nil {
		return ipcResult{}, err
	}
	if err := c.enc.Encode(msg); err != nil {
		return ipcResult{}, err
	}
	var res ipcMessage
	if err := c.dec.Decode(&res); err != nil {
		return ipcResult{}, err
	}
	if res.Type != "result" || res.TaskID != req.TaskID {
		return ipcResult{}, errIPCBadResponse
	}
	if res.ErrorCode != "" {
		return ipcResult{errorCode: res.ErrorCode, errorMessage: res.ErrorMessage}, nil
	}
	if res.ResultNode == "" {
		return ipcResult{}, errIPCBadResponse
	}
	nodeBytes, err := base64.StdEncoding.DecodeString(res.ResultNode)
	if err != nil {
		return ipcResult{}, err
	}
	return ipcResult{nodeBytes: nodeBytes}, nil
}

func rotateIPCToken() (string, error) {
	raw := make([]byte, ipcTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.StdEncoding.EncodeToString(raw)
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dirs.RunDir, 0o755); err != nil {
		return "", err
	}
	f, err := perm.OpenOwnerOnly(dirs.GatewayTokenPath())
	if err != nil {
		return "", errors.New("ERR_GATEWAY_UNAUTHORIZED")
	}
	defer func() { _ = f.Close() }()
	if err := f.Truncate(0); err != nil {
		return "", err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return "", err
	}
	if _, err := f.WriteString(token); err != nil {
		return "", err
	}
	return token, nil
}

type tasksRequest struct {
	TaskID          string
	ClientRequestID string
	JobType         string
	Input           json.RawMessage
	TSMs            int64
}

type ipcResult struct {
	nodeBytes    []byte
	errorCode    string
	errorMessage string
}
