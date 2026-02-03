package runtime

import (
	"time"

	"github.com/dianabuilds/ardents/internal/core/domain/garlic"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/envelopev2"
	"github.com/dianabuilds/ardents/internal/shared/lockeys"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func (r *Runtime) handleGarlic(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	msg, err := garlic.Decode(payload)
	if err != nil {
		return err
	}
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return err
	}
	keypair, err := lockeys.Load(dirs.LKeysDir(), msg.ToServiceID)
	if err != nil {
		return err
	}
	inner, err := garlic.Decrypt(msg, keypair.Private)
	if err != nil {
		return err
	}
	nowMs := timeutil.NowUnixMs()
	if inner.ExpiresAtMs > 0 && inner.ExpiresAtMs < nowMs {
		return garlic.ErrGarlicExpired
	}
	for _, clove := range inner.Cloves {
		if clove.Kind != "envelope" || len(clove.Envelope) == 0 {
			continue
		}
		env, err := envelopev2.DecodeEnvelope(clove.Envelope)
		if err != nil {
			return err
		}
		if err := env.ValidateBasic(nowMs); err != nil {
			return err
		}
		if env.From.IdentityID != "" {
			if err := env.VerifySignature(env.From.IdentityID); err != nil {
				return err
			}
		}
		ttl := time.Duration(env.TTLMs) * time.Millisecond
		if ttl < 10*time.Minute {
			ttl = 10 * time.Minute
		}
		if r.dedup != nil && r.dedup.SeenWithTTL(env.MsgID, ttl) {
			continue
		}
		r.log.Event("info", "garlic", "garlic.deliver", msg.ToServiceID, env.MsgID, "")
		if resps, err := r.handleEnvelopeV2(env); err == nil {
			if len(resps) > 0 {
				r.log.Event("info", "garlic", "garlic.reply.pending", msg.ToServiceID, env.MsgID, "")
			}
		}
	}
	return nil
}
