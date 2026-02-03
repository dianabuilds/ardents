package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/aichat"
	netdbsvc "github.com/dianabuilds/ardents/internal/core/app/services/netdb"
	"github.com/dianabuilds/ardents/internal/core/app/services/nodefetch"
	"github.com/dianabuilds/ardents/internal/core/app/services/serviceannounce"
	"github.com/dianabuilds/ardents/internal/core/app/services/servicedesc"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/core/domain/providers"
	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/lockeys"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

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
	if r.cfg.Limits.MaxMsgBytes > 0 && uint64(len(data)) > r.cfg.Limits.MaxMsgBytes {
		r.metrics.IncMsgRejected("ERR_MSG_TOO_LARGE")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_MSG_TOO_LARGE", fromPeerID)}, nil
	}
	if r.cfg.Limits.MaxPayloadBytes > 0 && uint64(len(env.Payload)) > r.cfg.Limits.MaxPayloadBytes {
		r.metrics.IncMsgRejected("ERR_PAYLOAD_TOO_LARGE")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_TOO_LARGE", fromPeerID)}, nil
	}
	if err := env.ValidateBasic(nowMs); err != nil {
		r.metrics.IncMsgRejected(mapError(err))
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", mapError(err), fromPeerID)}, nil
	}
	if r.IsBanned(fromPeerID) {
		r.metrics.IncMsgRejected("ERR_POW_INVALID")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_POW_INVALID", fromPeerID)}, nil
	}
	if env.Type == ackType {
		return nil, nil
	}
	if r.dedup.SeenWithTTL(env.MsgID, time.Duration(env.TTLMs)*time.Millisecond) {
		r.metrics.IncMsgRejected("ERR_DEDUP")
		return [][]byte{r.buildAck(env.MsgID, "DUPLICATE", "", fromPeerID)}, nil
	}
	if env.From.IdentityID != "" && len(env.Sig) == 0 {
		r.metrics.IncMsgRejected("ERR_SIG_REQUIRED")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SIG_REQUIRED", fromPeerID)}, nil
	}
	if env.From.IdentityID != "" {
		if r.book.IsRevokedIdentity(env.From.IdentityID) {
			r.metrics.IncMsgRejected("ERR_ID_REVOKED")
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_ID_REVOKED", fromPeerID)}, nil
		}
		if err := env.VerifySignature(env.From.IdentityID); err != nil {
			r.metrics.IncMsgRejected("ERR_SIG_INVALID")
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SIG_INVALID", fromPeerID)}, nil
		}
		trusted := r.book.IsTrustedIdentity(env.From.IdentityID, nowMs) && !r.book.IsDeprecatedIdentity(env.From.IdentityID)
		if !trusted {
			if env.Pow == nil {
				r.metrics.IncPowRequired()
				r.metrics.IncMsgRejected(pow.ErrPowRequired.Error())
				r.handlePowAbuse(fromPeerID)
				return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowRequired.Error(), fromPeerID)}, nil
			}
			env.Pow.Subject = pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
			if err := pow.Verify(env.Pow); err != nil {
				r.metrics.IncPowInvalid()
				r.metrics.IncMsgRejected(pow.ErrPowInvalid.Error())
				r.handlePowAbuse(fromPeerID)
				return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowInvalid.Error(), fromPeerID)}, nil
			}
			r.resetPowAbuse(fromPeerID)
		} else {
			r.resetPowAbuse(fromPeerID)
		}
	} else {
		if env.Pow == nil {
			r.metrics.IncPowRequired()
			r.metrics.IncMsgRejected(pow.ErrPowRequired.Error())
			r.handlePowAbuse(fromPeerID)
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowRequired.Error(), fromPeerID)}, nil
		}
		env.Pow.Subject = pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
		if err := pow.Verify(env.Pow); err != nil {
			r.metrics.IncPowInvalid()
			r.metrics.IncMsgRejected(pow.ErrPowInvalid.Error())
			r.handlePowAbuse(fromPeerID)
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowInvalid.Error(), fromPeerID)}, nil
		}
		r.resetPowAbuse(fromPeerID)
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
	if env.Type == netdbsvc.FindNodeType {
		req, err := netdbsvc.DecodeFindNode(env.Payload)
		if err != nil || req.V != netdbsvc.Version || len(req.Key) != 32 {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		nodes := []string{}
		if r.netdb != nil {
			nodes = r.netdb.FindClosestNodes(req.Key, r.netdb.K())
		}
		reply := netdbsvc.Reply{V: netdbsvc.Version, Status: "OK", Nodes: nodes}
		replyBytes, err := netdbsvc.EncodeReply(reply)
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		outID, err := uuidv7.New()
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
		}
		outEnv := envelope.Envelope{
			V:     envelope.Version,
			MsgID: outID,
			Type:  netdbsvc.ReplyType,
			From: envelope.From{
				PeerID: r.peerID,
			},
			To: envelope.To{
				PeerID: fromPeerID,
			},
			TSMs:    timeutil.NowUnixMs(),
			TTLMs:   int64((1 * time.Minute) / time.Millisecond),
			Payload: replyBytes,
		}
		encoded, err := outEnv.Encode()
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID), encoded}, nil
	}
	if env.Type == netdbsvc.FindValueType {
		req, err := netdbsvc.DecodeFindValue(env.Payload)
		if err != nil || req.V != netdbsvc.Version || len(req.Key) != 32 {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
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
		replyBytes, err := netdbsvc.EncodeReply(reply)
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		outID, err := uuidv7.New()
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
		}
		outEnv := envelope.Envelope{
			V:     envelope.Version,
			MsgID: outID,
			Type:  netdbsvc.ReplyType,
			From: envelope.From{
				PeerID: r.peerID,
			},
			To: envelope.To{
				PeerID: fromPeerID,
			},
			TSMs:    timeutil.NowUnixMs(),
			TTLMs:   int64((1 * time.Minute) / time.Millisecond),
			Payload: replyBytes,
		}
		encoded, err := outEnv.Encode()
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID), encoded}, nil
	}
	if env.Type == netdbsvc.StoreType {
		req, err := netdbsvc.DecodeStore(env.Payload)
		if err != nil || req.V != netdbsvc.Version || len(req.Value) == 0 {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
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
		replyBytes, err := netdbsvc.EncodeReply(reply)
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		outID, err := uuidv7.New()
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
		}
		outEnv := envelope.Envelope{
			V:     envelope.Version,
			MsgID: outID,
			Type:  netdbsvc.ReplyType,
			From: envelope.From{
				PeerID: r.peerID,
			},
			To: envelope.To{
				PeerID: fromPeerID,
			},
			TSMs:    timeutil.NowUnixMs(),
			TTLMs:   int64((1 * time.Minute) / time.Millisecond),
			Payload: replyBytes,
		}
		encoded, err := outEnv.Encode()
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID), encoded}, nil
	}
	if env.Type == netdbsvc.ReplyType {
		return nil, nil
	}
	r.metrics.IncMsgReceived(env.Type)
	if env.Type == nodefetch.ProviderAnnounceType {
		rec, err := nodefetch.DecodeProviderAnnounce(env.Payload)
		if err != nil || rec.NodeID == "" {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if rec.V != nodefetch.Version {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
		}
		if rec.ProviderPeerID == "" {
			rec.ProviderPeerID = fromPeerID
		}
		if rec.ProviderPeerID != fromPeerID {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if rec.TSMs == 0 {
			rec.TSMs = env.TSMs
		}
		if rec.TTLMs <= 0 {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
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
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, nil
	}
	if env.Type == serviceannounce.Type {
		ann, err := serviceannounce.Decode(env.Payload)
		if err != nil || ann.ServiceID == "" || ann.DescriptorNodeID == "" {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if ann.V != 1 || ann.TTLMs <= 0 {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
		}
		descBytes, err := r.FetchNode(context.Background(), ann.DescriptorNodeID)
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_DESCRIPTOR_INVALID", fromPeerID)}, nil
		}
		if err := contentnode.VerifyBytes(descBytes, ann.DescriptorNodeID); err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_DESCRIPTOR_INVALID", fromPeerID)}, nil
		}
		var node contentnode.Node
		if err := contentnode.Decode(descBytes, &node); err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_DESCRIPTOR_INVALID", fromPeerID)}, nil
		}
		body, err := servicedesc.Validate(node)
		if err != nil {
			if errors.Is(err, servicedesc.ErrServiceIDMismatch) {
				return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_ID_MISMATCH", fromPeerID)}, nil
			}
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_DESCRIPTOR_INVALID", fromPeerID)}, nil
		}
		if ann.ServiceID != body.ServiceID {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_ID_MISMATCH", fromPeerID)}, nil
		}
		if r.book.IsRevokedIdentity(node.Owner) {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_ID_REVOKED", fromPeerID)}, nil
		}
		if !r.book.IsTrustedIdentity(node.Owner, nowMs) || r.book.IsDeprecatedIdentity(node.Owner) {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_DESCRIPTOR_INVALID", fromPeerID)}, nil
		}
		if r.services != nil {
			if r.services.UpdateIfNewer(body.ServiceID, ann.DescriptorNodeID, node.CreatedAtMs, body, fromPeerID) {
				r.log.Event("info", "service", "service.descriptor.updated", body.ServiceID, ann.DescriptorNodeID, "")
			}
		}
		if node.Owner == r.identity.ID && hasLocalEndpoint(body.Endpoints, r.peerID) {
			r.registerLocalService(localServiceInfo{
				ServiceID:       body.ServiceID,
				ServiceName:     body.ServiceName,
				DescriptorV1CID: ann.DescriptorNodeID,
			})
			if dirs, err := appdirs.Resolve(""); err == nil {
				if _, err := lockeys.LoadOrCreate(dirs.LKeysDir(), body.ServiceID); err != nil {
					r.log.Event("warn", "service", "service.lkeys.ensure_failed", body.ServiceID, ann.DescriptorNodeID, err.Error())
				}
			}
			_ = r.publishServiceHeadAndLeaseSet(localServiceInfo{
				ServiceID:       body.ServiceID,
				ServiceName:     body.ServiceName,
				DescriptorV1CID: ann.DescriptorNodeID,
			})
		}
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, nil
	}
	if env.Type == tasks.RequestType {
		req, err := tasks.DecodeRequest(env.Payload)
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if req.V != tasks.Version || req.TaskID == "" || req.ClientRequestID == "" || req.JobType == "" || req.TSMs <= 0 {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if err := uuidv7.Validate(req.TaskID); err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if err := uuidv7.Validate(req.ClientRequestID); err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
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
				return resps, nil
			}
			nodeID, err := r.buildAITranscript(req.TaskID, input)
			if err != nil {
				fail := r.buildTaskFail(req.TaskID, err.Error(), "", fromPeerID)
				resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
				resps = append(resps, fail...)
				if r.tasks != nil {
					r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
				}
				return resps, nil
			}
			accept := r.buildTaskAccept(req.TaskID, fromPeerID)
			result := r.buildTaskResult(req.TaskID, nodeID, fromPeerID)
			resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
			resps = append(resps, accept...)
			resps = append(resps, result...)
			if r.tasks != nil {
				r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, resps[1:])
			}
			return resps, nil
		}
		fail := r.buildTaskFail(req.TaskID, tasks.ErrTaskUnsupported.Error(), "", fromPeerID)
		resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
		resps = append(resps, fail...)
		if r.tasks != nil {
			r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
		}
		return resps, nil
	}
	if env.Type == nodefetch.RequestType {
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
		outID, err := uuidv7.New()
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
		}
		outEnv := envelope.Envelope{
			V:     envelope.Version,
			MsgID: outID,
			Type:  nodefetch.ResponseType,
			From: envelope.From{
				PeerID: r.peerID,
			},
			To: envelope.To{
				PeerID: fromPeerID,
			},
			TSMs:    timeutil.NowUnixMs(),
			TTLMs:   int64((1 * time.Minute) / time.Millisecond),
			Payload: respBytes,
		}
		encoded, err := outEnv.Encode()
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		ackBytes := r.buildAck(env.MsgID, "OK", "", fromPeerID)
		return [][]byte{ackBytes, encoded}, nil
	}
	if env.Type == "chat.msg.v1" {
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, nil
	}
	return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
}
