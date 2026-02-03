package runtime

import (
	"time"

	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func (r *Runtime) buildSignedEnvelopeBytes(typ string, toPeerID string, payload []byte, ttlMs int64) ([]byte, error) {
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: "",
		Type:  typ,
		From: envelope.From{
			PeerID:     r.peerID,
			IdentityID: r.identity.ID,
		},
		To: envelope.To{
			PeerID: toPeerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   ttlMs,
		Payload: payload,
	}
	msgID, err := uuidv7.New()
	if err != nil {
		return nil, err
	}
	env.MsgID = msgID
	if r.identity.PrivateKey != nil && r.identity.ID != "" {
		if err := env.Sign(r.identity.PrivateKey); err != nil {
			return nil, err
		}
	} else {
		sub := pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
		stamp, err := pow.Generate(sub, r.cfg.Pow.DefaultDifficulty)
		if err != nil {
			return nil, err
		}
		env.Pow = stamp
	}
	return env.Encode()
}

func ttlMinuteMs() int64 {
	return int64((1 * time.Minute) / time.Millisecond)
}
