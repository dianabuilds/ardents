package runtime

import (
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/aichat"
	netdbsvc "github.com/dianabuilds/ardents/internal/core/app/services/netdb"
	"github.com/dianabuilds/ardents/internal/core/app/services/nodefetch"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/providers"
	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

var errReplyID = errors.New("ERR_REPLY_ID")

func (r *Runtime) handleEnvelope(fromPeerID string, data []byte) (resps [][]byte, err error) {
	r.capture("in", fromPeerID, data)
	defer func() {
		r.captureOutbound(fromPeerID, resps)
	}()
	env, err := envelope.DecodeEnvelope(data)
	if err != nil {
		return nil, err
	}
	nowMs := timeutil.NowUnixMs()
	if resps, handled := r.validateEnvelopeBasics(fromPeerID, env, data, nowMs); handled {
		return resps, nil
	}
	if env.Type == ackType {
		return nil, nil
	}
	if r.dedup.SeenWithTTL(env.MsgID, time.Duration(env.TTLMs)*time.Millisecond) {
		r.metrics.IncMsgRejected("ERR_DEDUP")
		return [][]byte{r.buildAck(env.MsgID, "DUPLICATE", "", fromPeerID)}, nil
	}
	if resps, handled := r.verifyEnvelopeAuth(fromPeerID, env, nowMs); handled {
		return resps, nil
	}
	if env.Type == relayType {
		r.metrics.IncMsgReceived(relayType)
		return r.handleRelay(fromPeerID, env, nowMs)
	}
	if env.Type == tunnel.BuildType {
		r.metrics.IncMsgReceived(tunnel.BuildType)
		return r.handleTunnelBuild(fromPeerID, env.Payload)
	}
	if env.Type == tunnel.DataType {
		r.metrics.IncMsgReceived(tunnel.DataType)
		return r.handleTunnelData(fromPeerID, env.Payload)
	}
	if env.Type == tunnel.BuildReplyType {
		return nil, nil
	}
	if resps, handled := r.handleNetDBMessage(fromPeerID, env, nowMs); handled {
		return resps, nil
	}
	r.metrics.IncMsgReceived(env.Type)
	if env.Type == nodefetch.ProviderAnnounceType {
		return r.handleProviderAnnounce(fromPeerID, env, nowMs), nil
	}
	if env.Type == tasks.RequestType {
		return r.handleTaskRequest(fromPeerID, env)
	}
	if env.Type == tasks.AcceptType || env.Type == tasks.ProgressType || env.Type == tasks.ResultType || env.Type == tasks.FailType || env.Type == tasks.ReceiptType {
		return r.handleTaskResponse(fromPeerID, env), nil
	}
	if env.Type == nodefetch.RequestType {
		return r.handleNodeFetch(fromPeerID, env)
	}
	return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
}

func (r *Runtime) requireValidPow(fromPeerID string, env *envelope.Envelope) ([][]byte, bool) {
	if env.Pow == nil {
		r.metrics.IncPowRequired()
		r.metrics.IncMsgRejected(pow.ErrPowRequired.Error())
		r.handlePowAbuse(fromPeerID)
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowRequired.Error(), fromPeerID)}, true
	}
	env.Pow.Subject = pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
	if err := pow.Verify(env.Pow); err != nil {
		r.metrics.IncPowInvalid()
		r.metrics.IncMsgRejected(pow.ErrPowInvalid.Error())
		r.handlePowAbuse(fromPeerID)
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowInvalid.Error(), fromPeerID)}, true
	}
	return nil, false
}

func (r *Runtime) validateEnvelopeBasics(fromPeerID string, env *envelope.Envelope, data []byte, nowMs int64) ([][]byte, bool) {
	if r.cfg.Limits.MaxMsgBytes > 0 && uint64(len(data)) > r.cfg.Limits.MaxMsgBytes {
		r.metrics.IncMsgRejected("ERR_MSG_TOO_LARGE")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_MSG_TOO_LARGE", fromPeerID)}, true
	}
	if r.cfg.Limits.MaxPayloadBytes > 0 && uint64(len(env.Payload)) > r.cfg.Limits.MaxPayloadBytes {
		r.metrics.IncMsgRejected("ERR_PAYLOAD_TOO_LARGE")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_TOO_LARGE", fromPeerID)}, true
	}
	if err := env.ValidateBasic(nowMs); err != nil {
		r.metrics.IncMsgRejected(mapError(err))
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", mapError(err), fromPeerID)}, true
	}
	if r.IsBanned(fromPeerID) {
		r.metrics.IncMsgRejected("ERR_POW_INVALID")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_POW_INVALID", fromPeerID)}, true
	}
	return nil, false
}

func (r *Runtime) verifyEnvelopeAuth(fromPeerID string, env *envelope.Envelope, nowMs int64) ([][]byte, bool) {
	if env.From.IdentityID != "" && len(env.Sig) == 0 {
		r.metrics.IncMsgRejected("ERR_SIG_REQUIRED")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SIG_REQUIRED", fromPeerID)}, true
	}
	if env.From.IdentityID != "" {
		if r.book.IsRevokedIdentity(env.From.IdentityID) {
			r.metrics.IncMsgRejected("ERR_ID_REVOKED")
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_ID_REVOKED", fromPeerID)}, true
		}
		if err := env.VerifySignature(env.From.IdentityID); err != nil {
			r.metrics.IncMsgRejected("ERR_SIG_INVALID")
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SIG_INVALID", fromPeerID)}, true
		}
		trusted := r.book.IsTrustedIdentity(env.From.IdentityID, nowMs) && !r.book.IsDeprecatedIdentity(env.From.IdentityID)
		if !trusted {
			if resps, handled := r.requireValidPow(fromPeerID, env); handled {
				return resps, true
			}
		}
		r.resetPowAbuse(fromPeerID)
		return nil, false
	}
	if resps, handled := r.requireValidPow(fromPeerID, env); handled {
		return resps, true
	}
	r.resetPowAbuse(fromPeerID)
	return nil, false
}

func (r *Runtime) handleNetDBMessage(fromPeerID string, env *envelope.Envelope, nowMs int64) ([][]byte, bool) {
	switch env.Type {
	case netdbsvc.FindNodeType:
		return r.handleNetDBFindNode(fromPeerID, env)
	case netdbsvc.FindValueType:
		return r.handleNetDBFindValue(fromPeerID, env)
	case netdbsvc.StoreType:
		return r.handleNetDBStore(fromPeerID, env, nowMs)
	case netdbsvc.ReplyType:
		return nil, true
	default:
		return nil, false
	}
}

func (r *Runtime) handleNetDBFindNode(fromPeerID string, env *envelope.Envelope) ([][]byte, bool) {
	req, ok := decodeNetDBFindNode(env.Payload)
	if !ok {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, true
	}
	var nodes []string
	if r.netdb != nil {
		nodes = r.netdb.FindClosestNodes(req.Key, r.netdb.K())
	}
	reply := netdbsvc.Reply{V: netdbsvc.Version, Status: "OK", Nodes: nodes}
	return r.buildNetDBReply(fromPeerID, env.MsgID, reply)
}

func (r *Runtime) handleNetDBFindValue(fromPeerID string, env *envelope.Envelope) ([][]byte, bool) {
	req, ok := decodeNetDBFindValue(env.Payload)
	if !ok {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, true
	}
	var value []byte
	if r.netdb != nil {
		if v, ok := r.netdb.FindValue(req.Key); ok {
			value = v
		}
	}
	reply := netdbsvc.Reply{V: netdbsvc.Version, Status: "NOT_FOUND"}
	if len(value) > 0 {
		reply.Status = "OK"
		reply.Value = value
	}
	return r.buildNetDBReply(fromPeerID, env.MsgID, reply)
}

func (r *Runtime) handleNetDBStore(fromPeerID string, env *envelope.Envelope, nowMs int64) ([][]byte, bool) {
	req, ok := decodeNetDBStore(env.Payload)
	if !ok {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, true
	}
	status := "REJECTED"
	code := "ERR_NETDB_BAD_RECORD"
	if r.netdb != nil {
		status, code = r.netdb.Store(req.Value, nowMs)
		if status == "OK" {
			r.log.Event("info", "netdb", "netdb.store.accepted", fromPeerID, env.MsgID, "")
		} else {
			r.log.Event("warn", "netdb", "netdb.store.rejected", fromPeerID, env.MsgID, code)
		}
	}
	reply := netdbsvc.Reply{V: netdbsvc.Version, Status: status, ErrorCode: code}
	return r.buildNetDBReply(fromPeerID, env.MsgID, reply)
}

func (r *Runtime) buildNetDBReply(fromPeerID string, msgID string, reply netdbsvc.Reply) ([][]byte, bool) {
	replyBytes, err := netdbsvc.EncodeReply(reply)
	if err != nil {
		return [][]byte{r.buildAck(msgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, true
	}
	resps, _ := r.buildAckAndReply(msgID, fromPeerID, netdbsvc.ReplyType, replyBytes)
	return resps, true
}

func decodeNetDBFindNode(payload []byte) (netdbsvc.FindNode, bool) {
	req, err := netdbsvc.DecodeFindNode(payload)
	if err != nil || req.V != netdbsvc.Version || len(req.Key) != 32 {
		return netdbsvc.FindNode{}, false
	}
	return req, true
}

func decodeNetDBFindValue(payload []byte) (netdbsvc.FindValue, bool) {
	req, err := netdbsvc.DecodeFindValue(payload)
	if err != nil || req.V != netdbsvc.Version || len(req.Key) != 32 {
		return netdbsvc.FindValue{}, false
	}
	return req, true
}

func decodeNetDBStore(payload []byte) (netdbsvc.Store, bool) {
	req, err := netdbsvc.DecodeStore(payload)
	if err != nil || req.V != netdbsvc.Version || len(req.Value) == 0 {
		return netdbsvc.Store{}, false
	}
	return req, true
}

func (r *Runtime) handleProviderAnnounce(fromPeerID string, env *envelope.Envelope, nowMs int64) [][]byte {
	rec, err := nodefetch.DecodeProviderAnnounce(env.Payload)
	if err != nil || rec.NodeID == "" {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}
	}
	if rec.V != nodefetch.Version {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}
	}
	if rec.ProviderPeerID == "" {
		rec.ProviderPeerID = fromPeerID
	}
	if rec.ProviderPeerID != fromPeerID {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}
	}
	if rec.TSMs == 0 {
		rec.TSMs = env.TSMs
	}
	if rec.TTLMs <= 0 {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}
	}
	if r.providers != nil {
		r.providers.Add(providers.ProviderRecord{
			V:              rec.V,
			NodeID:         rec.NodeID,
			ProviderPeerID: rec.ProviderPeerID,
			TSMs:           rec.TSMs,
			TTLMs:          rec.TTLMs,
		}, nowMs)
	}
	return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
}

func (r *Runtime) handleTaskRequest(fromPeerID string, env *envelope.Envelope) ([][]byte, error) {
	req, err := decodeTaskRequestPayload(env.Payload)
	if err != nil {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
	}
	if r.metrics != nil {
		r.metrics.IncTaskRequested(req.JobType)
	}
	if r.tasks != nil {
		if dup, errCode := r.tasks.Check(req.TaskID, req.ClientRequestID, env.Payload); errCode != "" {
			fail := r.buildTaskFail(req.TaskID, errCode, "", fromPeerID)
			return append([][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, fail...), nil
		} else if len(dup) > 0 {
			resps := make([][]byte, 0, len(dup)+1)
			resps = append(resps, r.buildAck(env.MsgID, "OK", "", fromPeerID))
			resps = append(resps, dup...)
			return resps, nil
		}
	}
	if req.JobType == "ai.chat.v1" {
		return r.handleAIChatTask(fromPeerID, env, req), nil
	}
	if r.ipc != nil {
		if resps, ok := r.handleTaskIPC(req, env, fromPeerID); ok {
			return resps, nil
		}
	}
	fail := r.buildTaskFail(req.TaskID, tasks.ErrTaskUnsupported.Error(), "", fromPeerID)
	resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
	resps = append(resps, fail...)
	if r.tasks != nil {
		r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
	}
	return resps, nil
}

func (r *Runtime) handleAIChatTask(fromPeerID string, env *envelope.Envelope, req tasks.Request) [][]byte {
	input, err := aichat.DecodeInput(req.Input, r.cfg.Limits.MaxPayloadBytes)
	if err != nil {
		code := err.Error()
		if code == aichat.ErrInputInvalid.Error() {
			code = "ERR_PAYLOAD_DECODE"
		}
		fail := r.buildTaskFail(req.TaskID, code, "", fromPeerID)
		resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
		resps = append(resps, fail...)
		if r.tasks != nil {
			r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
		}
		return resps
	}
	nodeID, err := r.buildAITranscript(req.TaskID, input)
	if err != nil {
		fail := r.buildTaskFail(req.TaskID, err.Error(), "", fromPeerID)
		resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
		resps = append(resps, fail...)
		if r.tasks != nil {
			r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
		}
		return resps
	}
	return r.buildTaskSuccessResps(env, req, fromPeerID, nodeID)
}

func (r *Runtime) handleNodeFetch(fromPeerID string, env *envelope.Envelope) ([][]byte, error) {
	req, err := nodefetch.DecodeRequest(env.Payload)
	if err != nil || req.NodeID == "" {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
	}
	if r.store == nil {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", nodefetch.ErrNodeNotFound.Error(), fromPeerID)}, nil
	}
	nodeBytes, err := r.store.Get(req.NodeID)
	if err != nil {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", nodefetch.ErrNodeNotFound.Error(), fromPeerID)}, nil
	}
	resp := nodefetch.Response{V: nodefetch.Version, NodeBytes: nodeBytes}
	respBytes, err := nodefetch.EncodeResponse(resp)
	if err != nil {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
	}
	return r.buildAckAndReply(env.MsgID, fromPeerID, nodefetch.ResponseType, respBytes)
}

func (r *Runtime) buildAckAndReply(msgID string, fromPeerID string, replyType string, payload []byte) ([][]byte, error) {
	encoded, err := r.buildReplyEnvelope(fromPeerID, replyType, payload)
	if err != nil {
		code := "ERR_PAYLOAD_DECODE"
		if errors.Is(err, errReplyID) {
			code = "ERR_UNSUPPORTED_TYPE"
		}
		return [][]byte{r.buildAck(msgID, "REJECTED", code, fromPeerID)}, nil
	}
	return [][]byte{r.buildAck(msgID, "OK", "", fromPeerID), encoded}, nil
}

func (r *Runtime) buildReplyEnvelope(toPeerID string, typ string, payload []byte) ([]byte, error) {
	outID, err := uuidv7.New()
	if err != nil {
		return nil, errReplyID
	}
	outEnv := envelope.Envelope{
		V:     envelope.Version,
		MsgID: outID,
		Type:  typ,
		From: envelope.From{
			PeerID: r.peerID,
		},
		To: envelope.To{
			PeerID: toPeerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
	}
	encoded, err := outEnv.Encode()
	if err != nil {
		return nil, err
	}
	return encoded, nil
}
