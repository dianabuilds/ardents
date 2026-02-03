package main

import (
	"context"
	"errors"
	"fmt"
	mrand "math/rand"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/app/runtime"
	"github.com/dianabuilds/ardents/internal/core/app/services/dirquery"
	"github.com/dianabuilds/ardents/internal/core/app/services/servicedesc"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/core/domain/garlic"
	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/envelopev2"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/lockeys"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func checkDirQueryE2E(rng *mrand.Rand) error {
	net := newSimNetwork(5)
	if err := net.init(); err != nil {
		return err
	}
	dir := net.peers[0]
	client := net.peers[1]
	ctx := context.Background()
	if err := ensureDirQueryTunnels(dir, client, ctx); err != nil {
		return err
	}
	keys, err := loadDirQueryKeys(dir, client)
	if err != nil {
		return err
	}
	if err := registerDirQueryService(dir); err != nil {
		return err
	}
	inSnap := client.SimV2InboundSnapshot()
	if err := publishMailboxLease(net, client, inSnap, keys.mailboxID, keys.mailboxKeys); err != nil {
		return err
	}
	captured, outSnap, err := captureDirQueryReply(net, dir)
	if err != nil {
		return err
	}
	if err := sendDirQueryRequest(dir, client, keys, captured); err != nil {
		return err
	}
	if err := validateDirQueryReply(dir, client, keys, outSnap, *captured); err != nil {
		return err
	}
	_ = rng
	return nil
}

type dirQueryKeys struct {
	dirServiceID string
	dirKeys      lockeys.Keypair
	mailboxID    string
	mailboxKeys  lockeys.Keypair
}

func ensureDirQueryTunnels(dir *runtime.Runtime, client *runtime.Runtime, ctx context.Context) error {
	if err := dir.SimV2RotateTunnels(ctx); err != nil {
		return err
	}
	if err := client.SimV2RotateTunnels(ctx); err != nil {
		return err
	}
	inSnap := client.SimV2InboundSnapshot()
	if inSnap == nil || len(inSnap.HopPeerIDs) == 0 {
		return errors.New("ERR_SIM_INBOUND_TUNNEL_MISSING")
	}
	return nil
}

func loadDirQueryKeys(dir *runtime.Runtime, client *runtime.Runtime) (dirQueryKeys, error) {
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return dirQueryKeys{}, err
	}
	dirServiceID, err := ids.NewServiceID(dir.IdentityID(), dirquery.JobType)
	if err != nil {
		return dirQueryKeys{}, err
	}
	dirKeys, err := lockeys.LoadOrCreate(dirs.LKeysDir(), dirServiceID)
	if err != nil {
		return dirQueryKeys{}, err
	}
	mailboxName := "mailbox.msg.v1"
	mailboxID, err := ids.NewServiceID(client.IdentityID(), mailboxName)
	if err != nil {
		return dirQueryKeys{}, err
	}
	mailboxKeys, err := lockeys.LoadOrCreate(dirs.LKeysDir(), mailboxID)
	if err != nil {
		return dirQueryKeys{}, err
	}
	return dirQueryKeys{
		dirServiceID: dirServiceID,
		dirKeys:      dirKeys,
		mailboxID:    mailboxID,
		mailboxKeys:  mailboxKeys,
	}, nil
}

func registerDirQueryService(dir *runtime.Runtime) error {
	caps := []servicedesc.Capability{
		{V: 1, JobType: dirquery.JobType, Modes: []string{"v2"}},
	}
	descNode, descCID, err := servicedesc.BuildDescriptorNodeV2(dir.IdentityID(), dir.IdentityPrivateKey(), dirquery.JobType, caps, map[string]uint64{}, map[string]uint64{})
	if err != nil {
		return err
	}
	descBytes, _, err := contentnode.EncodeWithCID(descNode)
	if err != nil {
		return err
	}
	if err := dir.Store().Put(descCID, descBytes); err != nil {
		return err
	}
	if err := dir.SimV2RegisterLocalService(dirquery.JobType, descCID); err != nil {
		return err
	}
	return dir.SimV2PublishLocalServices()
}

func publishMailboxLease(net *simNetwork, client *runtime.Runtime, inSnap *runtime.TunnelPathSnapshot, mailboxID string, mailboxKeys lockeys.Keypair) error {
	if inSnap == nil || len(inSnap.HopPeerIDs) == 0 {
		return errors.New("ERR_SIM_INBOUND_TUNNEL_MISSING")
	}
	nowMs := timeutil.NowUnixMs()
	leaseSet := netdb.LeaseSet{
		V:               1,
		ServiceID:       mailboxID,
		OwnerIdentityID: client.IdentityID(),
		ServiceName:     "mailbox.msg.v1",
		EncPub:          mailboxKeys.Public,
		Leases: []netdb.Lease{
			{
				GatewayPeerID: inSnap.HopPeerIDs[0],
				TunnelID:      inSnap.HopTunnelIDs[0],
				ExpiresAtMs:   nowMs + 120_000,
			},
		},
		PublishedAtMs: nowMs,
		ExpiresAtMs:   nowMs + 120_000,
	}
	leaseSet, err := netdb.SignLeaseSet(client.IdentityPrivateKey(), leaseSet)
	if err != nil {
		return err
	}
	leaseBytes, err := netdb.EncodeLeaseSet(leaseSet)
	if err != nil {
		return err
	}
	if status, code := net.db.Store(leaseBytes, nowMs); status != "OK" {
		return fmt.Errorf("lease set store failed: %s", code)
	}
	return nil
}

func captureDirQueryReply(net *simNetwork, dir *runtime.Runtime) (*[]byte, *runtime.TunnelPathSnapshot, error) {
	var captured []byte
	outSnap := dir.SimV2OutboundSnapshot()
	if outSnap == nil || len(outSnap.HopPeerIDs) == 0 {
		return nil, nil, errors.New("ERR_SIM_OUTBOUND_TUNNEL_MISSING")
	}
	gatewayID := outSnap.HopPeerIDs[0]
	dir.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		if peerID == gatewayID && len(captured) == 0 {
			captured = envBytes
		}
		target, ok := net.byPeer[peerID]
		if !ok {
			return errors.New("ERR_SIM_PEER_MISSING")
		}
		_, _ = target.HandleEnvelope(dir.PeerID(), envBytes)
		return nil
	})
	return &captured, outSnap, nil
}

func sendDirQueryRequest(dir *runtime.Runtime, client *runtime.Runtime, keys dirQueryKeys, captured *[]byte) error {
	reqBytes, err := buildDirQueryRequest(client, keys.dirServiceID, keys.mailboxID)
	if err != nil {
		return err
	}
	inner := garlic.Inner{
		V:           garlic.Version,
		ExpiresAtMs: timeutil.NowUnixMs() + 60_000,
		Cloves: []garlic.Clove{
			{Kind: "envelope", Envelope: reqBytes},
		},
	}
	msg, err := garlic.Encrypt(keys.dirServiceID, keys.dirKeys.Public, inner)
	if err != nil {
		return err
	}
	msgBytes, err := garlic.Encode(msg)
	if err != nil {
		return err
	}
	if err := dir.SimV2HandleGarlic(msgBytes); err != nil {
		return err
	}
	if len(*captured) == 0 {
		return errors.New("ERR_SIM_OUTBOUND_REPLY_MISSING")
	}
	return nil
}

func buildDirQueryRequest(client *runtime.Runtime, dirServiceID string, mailboxID string) ([]byte, error) {
	nowMs := timeutil.NowUnixMs()
	taskID, _ := uuidv7.New()
	reqID, _ := uuidv7.New()
	req := tasks.Request{
		V:               tasks.Version,
		TaskID:          taskID,
		ClientRequestID: reqID,
		JobType:         dirquery.JobType,
		Input: map[string]any{
			"v":     uint64(dirquery.Version),
			"limit": uint64(1),
		},
		TSMs: nowMs,
	}
	reqBytes, err := tasks.EncodeRequest(req)
	if err != nil {
		return nil, err
	}
	env := envelopev2.Envelope{
		V:     envelopev2.Version,
		MsgID: taskID,
		Type:  tasks.RequestType,
		From:  envelopev2.From{IdentityID: client.IdentityID()},
		To:    envelopev2.To{ServiceID: dirServiceID},
		ReplyTo: &envelopev2.Reply{
			ServiceID: mailboxID,
		},
		TSMs:    nowMs,
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: reqBytes,
	}
	if err := env.Sign(client.IdentityPrivateKey()); err != nil {
		return nil, err
	}
	return env.Encode()
}

func validateDirQueryReply(dir *runtime.Runtime, client *runtime.Runtime, keys dirQueryKeys, outSnap *runtime.TunnelPathSnapshot, captured []byte) error {
	replyEnv, err := envelope.DecodeEnvelope(captured)
	if err != nil || replyEnv.Type != tunnel.DataType {
		return errors.New("ERR_SIM_EXPECTED_TUNNEL_DATA_REPLY")
	}
	garlicMsgBytes, err := peelTunnelToGarlic(replyEnv.Payload, outSnap)
	if err != nil {
		return err
	}
	replyGarlic, err := garlic.Decode(garlicMsgBytes)
	if err != nil {
		return err
	}
	replyInner, err := garlic.Decrypt(replyGarlic, keys.mailboxKeys.Private)
	if err != nil {
		return err
	}
	if len(replyInner.Cloves) == 0 {
		return errors.New("ERR_SIM_GARLIC_EMPTY")
	}
	replyClove := replyInner.Cloves[0]
	replyV2, err := envelopev2.DecodeEnvelope(replyClove.Envelope)
	if err != nil {
		return err
	}
	if replyV2.Type != tasks.AcceptType && replyV2.Type != tasks.ResultType && replyV2.Type != tasks.FailType {
		return errors.New("ERR_SIM_REPLY_TYPE_UNEXPECTED")
	}
	if err := client.SimV2HandleEnvelopeV2Bytes(replyClove.Envelope); err != nil {
		return err
	}
	if clientTasksSnapshot(client, replyV2) == nil {
		return errors.New("ERR_SIM_TASK_RESPONSE_NOT_STORED")
	}
	if replyV2.Type == tasks.ResultType {
		return validateDirQueryResultNode(dir, replyV2.Payload)
	}
	return nil
}

func validateDirQueryResultNode(dir *runtime.Runtime, payload []byte) error {
	res, err := tasks.DecodeResult(payload)
	if err != nil || res.ResultNodeID == "" {
		return errors.New("ERR_SIM_TASK_RESULT_INVALID")
	}
	nodeBytes, err := dir.Store().Get(res.ResultNodeID)
	if err != nil {
		return errors.New("ERR_SIM_RESULT_NODE_MISSING")
	}
	var node contentnode.Node
	if err := contentnode.Decode(nodeBytes, &node); err != nil {
		return err
	}
	if node.Type != dirquery.NodeType {
		return errors.New("ERR_SIM_RESULT_NODE_TYPE_UNEXPECTED")
	}
	return nil
}

func clientTasksSnapshot(rt *runtime.Runtime, env *envelopev2.Envelope) [][]byte {
	if rt == nil || env == nil {
		return nil
	}
	taskID := ""
	switch env.Type {
	case tasks.AcceptType:
		if a, err := tasks.DecodeAccept(env.Payload); err == nil {
			taskID = a.TaskID
		}
	case tasks.ProgressType:
		if p, err := tasks.DecodeProgress(env.Payload); err == nil {
			taskID = p.TaskID
		}
	case tasks.ResultType:
		if r, err := tasks.DecodeResult(env.Payload); err == nil {
			taskID = r.TaskID
		}
	case tasks.FailType:
		if f, err := tasks.DecodeFail(env.Payload); err == nil {
			taskID = f.TaskID
		}
	case tasks.ReceiptType:
		if r, err := tasks.DecodeReceipt(env.Payload); err == nil {
			taskID = r.TaskID
		}
	}
	if taskID == "" || rt.Tasks() == nil {
		return nil
	}
	return rt.Tasks().Responses(taskID)
}
