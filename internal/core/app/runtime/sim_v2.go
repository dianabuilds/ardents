package runtime

import (
	"context"
	"errors"

	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/shared/envelopev2"
	"github.com/dianabuilds/ardents/internal/shared/ids"
)

type TunnelPathSnapshot struct {
	HopPeerIDs   []string
	HopTunnelIDs [][]byte
	HopKeys      [][]byte
}

func (r *Runtime) SimV2RotateTunnels(ctx context.Context) error {
	if r == nil {
		return errors.New("ERR_SIM_RUNTIME_NIL")
	}
	r.rotateTunnels(ctx)
	return nil
}

func (r *Runtime) SimV2OutboundSnapshot() *TunnelPathSnapshot {
	if r == nil {
		return nil
	}
	r.tunnelMgrMu.Lock()
	defer r.tunnelMgrMu.Unlock()
	if len(r.outboundTunnels) == 0 {
		return nil
	}
	return snapshotPath(r.outboundTunnels[0])
}

func (r *Runtime) SimV2InboundSnapshot() *TunnelPathSnapshot {
	if r == nil {
		return nil
	}
	r.tunnelMgrMu.Lock()
	defer r.tunnelMgrMu.Unlock()
	if len(r.inboundTunnels) == 0 {
		return nil
	}
	return snapshotPath(r.inboundTunnels[0])
}

func (r *Runtime) SimV2BuildPaddingData() ([]byte, error) {
	if r == nil {
		return nil, errors.New("ERR_SIM_RUNTIME_NIL")
	}
	r.tunnelMgrMu.Lock()
	defer r.tunnelMgrMu.Unlock()
	if len(r.outboundTunnels) == 0 {
		return nil, errors.New("ERR_SIM_NO_OUTBOUND_TUNNEL")
	}
	return r.buildPaddingData(r.outboundTunnels[0])
}

func (r *Runtime) SimV2OnionPublic() []byte {
	if r == nil {
		return nil
	}
	out := append([]byte(nil), r.onionKey.Public...)
	return out
}

func (r *Runtime) SimV2HandleGarlic(payload []byte) error {
	if r == nil {
		return errors.New("ERR_SIM_RUNTIME_NIL")
	}
	return r.handleGarlic(payload)
}

func (r *Runtime) SimV2HandleEnvelopeV2Bytes(envBytes []byte) error {
	if r == nil {
		return errors.New("ERR_SIM_RUNTIME_NIL")
	}
	env, err := envelopev2.DecodeEnvelope(envBytes)
	if err != nil {
		return err
	}
	_, err = r.handleEnvelopeV2(env)
	return err
}

func (r *Runtime) SimV2RegisterLocalService(serviceName string, descriptorV2CID string) error {
	if r == nil {
		return errors.New("ERR_SIM_RUNTIME_NIL")
	}
	if serviceName == "" {
		return errors.New("ERR_SIM_SERVICE_NAME")
	}
	if r.identity.ID == "" {
		return errors.New("ERR_SIM_IDENTITY")
	}
	if _, err := ids.NewServiceID(r.identity.ID, serviceName); err != nil {
		return err
	}
	r.registerLocalService(localServiceInfo{
		ServiceName:     serviceName,
		DescriptorV2CID: descriptorV2CID,
	})
	return nil
}

func (r *Runtime) SimV2PublishLocalServices() error {
	if r == nil {
		return errors.New("ERR_SIM_RUNTIME_NIL")
	}
	r.publishLocalServices()
	return nil
}

func snapshotPath(path *tunnelPath) *TunnelPathSnapshot {
	if path == nil {
		return nil
	}
	out := &TunnelPathSnapshot{
		HopPeerIDs:   make([]string, 0, len(path.hops)),
		HopTunnelIDs: make([][]byte, 0, len(path.hops)),
		HopKeys:      make([][]byte, 0, len(path.hops)),
	}
	for _, hop := range path.hops {
		out.HopPeerIDs = append(out.HopPeerIDs, hop.peerID)
		out.HopTunnelIDs = append(out.HopTunnelIDs, append([]byte(nil), hop.tunnelID...))
		out.HopKeys = append(out.HopKeys, append([]byte(nil), hop.key...))
	}
	return out
}

func SimV2PeelPadding(data []byte, snap *TunnelPathSnapshot) (string, error) {
	if len(data) == 0 || snap == nil {
		return "", errors.New("ERR_SIM_PADDING_EMPTY")
	}
	curr := data
	for i := 0; i < len(snap.HopKeys); i++ {
		decoded, err := tunnel.DecodeData(curr)
		if err != nil {
			return "", err
		}
		inner, err := tunnel.DecryptData(snap.HopKeys[i], decoded.CT)
		if err != nil {
			return "", err
		}
		if inner.Kind != "forward" {
			return inner.Kind, nil
		}
		if len(inner.Inner) == 0 {
			return inner.Kind, errors.New("ERR_SIM_PADDING_MISSING")
		}
		curr = inner.Inner
	}
	return "", errors.New("ERR_SIM_PADDING_INCOMPLETE")
}
