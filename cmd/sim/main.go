package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"time"

	"github.com/dianabuilds/ardents/internal/addressbook"
	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/contentnode"
	"github.com/dianabuilds/ardents/internal/runtime"
	"github.com/dianabuilds/ardents/internal/services/nodefetch"
	"github.com/dianabuilds/ardents/internal/services/tasks"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

type simPeer struct {
	rt        *runtime.Runtime
	peerID    string
	identity  identity.Identity
	nodeID    string
	nodeBytes []byte
}

type stats struct {
	Sent          int
	Delivered     int
	Dropped       int
	AckOK         int
	AckRejected   int
	PowRequired   int
	PowInvalid    int
	AckByError    map[string]int
	LatencyMillis []int64
	ByType        map[string]int
}

func main() {
	fs := flag.NewFlagSet("sim", flag.ExitOnError)
	nPeers := fs.Int("n", 5, "number of peers")
	duration := fs.Duration("duration", 10*time.Second, "simulation duration")
	rate := fs.Int("rate", 10, "messages per second")
	seed := fs.Int64("seed", time.Now().UTC().UnixNano(), "random seed")
	dropRate := fs.Float64("drop-rate", 0, "drop rate 0..1")
	powInvalidRate := fs.Float64("pow-invalid-rate", 0, "rate of invalid/missing PoW 0..1")
	powDifficulty := fs.Uint64("pow-difficulty", 16, "PoW difficulty")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fatal(err)
	}
	if *nPeers < 2 {
		fatal(errors.New("n must be >= 2"))
	}
	if *rate <= 0 {
		fatal(errors.New("rate must be > 0"))
	}
	if *dropRate < 0 || *dropRate > 1 || *powInvalidRate < 0 || *powInvalidRate > 1 {
		fatal(errors.New("rates must be 0..1"))
	}

	rng := rand.New(rand.NewSource(*seed))
	cfg := config.Default()
	cfg.Pow.DefaultDifficulty = *powDifficulty

	peers := make([]simPeer, 0, *nPeers)
	for i := 0; i < *nPeers; i++ {
		id, err := identity.NewEphemeral()
		if err != nil {
			fatal(err)
		}
		peerID, err := ids.NewPeerID(id.PublicKey)
		if err != nil {
			fatal(err)
		}
		book := addressbook.Book{
			V:           1,
			Entries:     []addressbook.Entry{},
			UpdatedAtMs: timeutil.NowUnixMs(),
		}
		book.Entries = append(book.Entries, addressbook.Entry{
			Alias:       "self",
			TargetType:  "identity",
			TargetID:    id.ID,
			Source:      "self",
			Trust:       "trusted",
			CreatedAtMs: timeutil.NowUnixMs(),
		})
		rt := runtime.NewSim(cfg, peerID, id, book)
		nodeBytes, nodeID, err := buildSampleNode(id)
		if err != nil {
			fatal(err)
		}
		if err := rt.Store().Put(nodeID, nodeBytes); err != nil {
			fatal(err)
		}
		peers = append(peers, simPeer{rt: rt, peerID: peerID, identity: id, nodeID: nodeID, nodeBytes: nodeBytes})
	}

	st := stats{ByType: map[string]int{}, AckByError: map[string]int{}}
	interval := time.Second / time.Duration(*rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	end := time.Now().Add(*duration)
	for time.Now().Before(end) {
		<-ticker.C
		senderIdx := rng.Intn(len(peers))
		recvIdx := rng.Intn(len(peers))
		for recvIdx == senderIdx {
			recvIdx = rng.Intn(len(peers))
		}
		sender := peers[senderIdx]
		receiver := peers[recvIdx]
		msgType := pickType(rng)
		switch msgType {
		case "chat.msg.v1":
			st.ByType[msgType]++
			sendChat(rng, sender, receiver, &st, *dropRate, *powInvalidRate, *powDifficulty)
		case "task.request.v1":
			st.ByType[msgType]++
			sendTask(rng, sender, receiver, &st, *dropRate, *powInvalidRate, *powDifficulty)
		case "node.fetch.v1":
			st.ByType[msgType]++
			sendFetch(rng, sender, receiver, &st, *dropRate, *powInvalidRate, *powDifficulty)
		}
	}

	printStats(st)
}

func pickType(rng *rand.Rand) string {
	n := rng.Intn(100)
	switch {
	case n < 50:
		return "chat.msg.v1"
	case n < 80:
		return "task.request.v1"
	default:
		return "node.fetch.v1"
	}
}

func sendChat(rng *rand.Rand, sender simPeer, receiver simPeer, st *stats, dropRate, powInvalidRate float64, powDifficulty uint64) {
	payload, err := codec.Marshal(runtime.ChatMessage{V: 1, Text: "hello"})
	if err != nil {
		return
	}
	envBytes, err := buildEnvelope(sender, receiver, "chat.msg.v1", payload, powInvalidRate, powDifficulty, rng)
	if err != nil {
		return
	}
	deliverEnvelope(sender, receiver, envBytes, st, dropRate, rng)
}

func sendTask(rng *rand.Rand, sender simPeer, receiver simPeer, st *stats, dropRate, powInvalidRate float64, powDifficulty uint64) {
	taskID, err := uuidv7.New()
	if err != nil {
		return
	}
	reqID, err := uuidv7.New()
	if err != nil {
		return
	}
	input := map[string]any{
		"v": 1,
		"messages": []map[string]any{
			{"role": "user", "content": "ping"},
		},
		"policy": map[string]any{
			"visibility": "public",
		},
	}
	req := tasks.Request{
		V:               tasks.Version,
		TaskID:          taskID,
		ClientRequestID: reqID,
		JobType:         "ai.chat.v1",
		Input:           input,
		TSMs:            timeutil.NowUnixMs(),
	}
	payload, err := tasks.EncodeRequest(req)
	if err != nil {
		return
	}
	envBytes, err := buildEnvelope(sender, receiver, tasks.RequestType, payload, powInvalidRate, powDifficulty, rng)
	if err != nil {
		return
	}
	deliverEnvelope(sender, receiver, envBytes, st, dropRate, rng)
}

func sendFetch(rng *rand.Rand, sender simPeer, receiver simPeer, st *stats, dropRate, powInvalidRate float64, powDifficulty uint64) {
	reqBytes, err := nodefetch.EncodeRequest(nodefetch.Request{V: nodefetch.Version, NodeID: receiver.nodeID})
	if err != nil {
		return
	}
	envBytes, err := buildEnvelope(sender, receiver, nodefetch.RequestType, reqBytes, powInvalidRate, powDifficulty, rng)
	if err != nil {
		return
	}
	resps, ok := deliverEnvelope(sender, receiver, envBytes, st, dropRate, rng)
	if !ok {
		return
	}
	for _, resp := range resps {
		respEnv, err := envelope.DecodeEnvelope(resp)
		if err != nil || respEnv.Type != nodefetch.ResponseType {
			continue
		}
		respPayload, err := nodefetch.DecodeResponse(respEnv.Payload)
		if err != nil {
			continue
		}
		_ = contentnode.VerifyBytes(respPayload.NodeBytes, receiver.nodeID)
	}
}

func buildEnvelope(sender simPeer, receiver simPeer, typ string, payload []byte, powInvalidRate float64, powDifficulty uint64, rng *rand.Rand) ([]byte, error) {
	msgID, err := uuidv7.New()
	if err != nil {
		return nil, err
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  typ,
		From: envelope.From{
			PeerID:     sender.peerID,
			IdentityID: sender.identity.ID,
		},
		To: envelope.To{
			PeerID: receiver.peerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
	}
	if rng.Float64() < powInvalidRate {
		if rng.Intn(2) == 0 {
			env.Pow = nil
		} else {
			env.Pow = &pow.Stamp{
				V:          1,
				Difficulty: powDifficulty,
				Nonce:      []byte{0x01, 0x02},
				Subject:    make([]byte, 32),
			}
		}
	} else {
		sub := pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
		stamp, err := pow.Generate(sub, powDifficulty)
		if err != nil {
			return nil, err
		}
		env.Pow = stamp
	}
	if sender.identity.PrivateKey != nil {
		if err := env.Sign(sender.identity.PrivateKey); err != nil {
			return nil, err
		}
	}
	return env.Encode()
}

func deliverEnvelope(sender simPeer, receiver simPeer, envBytes []byte, st *stats, dropRate float64, rng *rand.Rand) ([][]byte, bool) {
	st.Sent++
	if rng.Float64() < dropRate {
		st.Dropped++
		return nil, false
	}
	start := time.Now()
	resps, err := receiver.rt.HandleEnvelope(sender.peerID, envBytes)
	if err != nil {
		return nil, false
	}
	st.Delivered++
	lat := time.Since(start).Milliseconds()
	if lat > 0 {
		st.LatencyMillis = append(st.LatencyMillis, lat)
	}
	for _, resp := range resps {
		env, err := envelope.DecodeEnvelope(resp)
		if err != nil || env.Type != "ack.v1" {
			continue
		}
		p, err := ack.Decode(env.Payload)
		if err != nil {
			continue
		}
		switch p.Status {
		case "OK", "DUPLICATE":
			st.AckOK++
		case "REJECTED":
			st.AckRejected++
			if p.ErrorCode != "" {
				st.AckByError[p.ErrorCode]++
			}
			if p.ErrorCode == pow.ErrPowRequired.Error() {
				st.PowRequired++
			}
			if p.ErrorCode == pow.ErrPowInvalid.Error() {
				st.PowInvalid++
			}
		}
	}
	return resps, true
}

func buildSampleNode(id identity.Identity) ([]byte, string, error) {
	node := contentnode.Node{
		V:           1,
		Type:        "sample.node.v1",
		CreatedAtMs: timeutil.NowUnixMs(),
		Owner:       id.ID,
		Links:       []contentnode.Link{},
		Body: map[string]any{
			"v":    1,
			"note": "sample",
		},
		Policy: map[string]any{
			"visibility": "public",
		},
	}
	if err := contentnode.Sign(&node, id.PrivateKey); err != nil {
		return nil, "", err
	}
	return contentnode.EncodeWithCID(node)
}

func printStats(st stats) {
	avg, p95 := latencyStats(st.LatencyMillis)
	out := map[string]any{
		"sent":            st.Sent,
		"delivered":       st.Delivered,
		"dropped":         st.Dropped,
		"drop_rate":       ratio(st.Dropped, st.Sent),
		"ack_ok":          st.AckOK,
		"ack_rejected":    st.AckRejected,
		"ack_rejected_by": st.AckByError,
		"pow_required":    st.PowRequired,
		"pow_invalid":     st.PowInvalid,
		"pow_reject_rate": ratio(st.PowRequired+st.PowInvalid, st.Delivered),
		"latency_avg_ms":  avg,
		"latency_p95_ms":  p95,
		"traffic_by_type": st.ByType,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}

func latencyStats(samples []int64) (avg int64, p95 int64) {
	if len(samples) == 0 {
		return 0, 0
	}
	total := int64(0)
	for _, v := range samples {
		total += v
	}
	avg = total / int64(len(samples))
	cp := append([]int64(nil), samples...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := int(float64(len(cp)-1) * 0.95)
	p95 = cp[idx]
	return avg, p95
}

func ratio(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
