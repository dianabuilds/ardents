package runtime

import (
	"context"
	"crypto/sha256"
	"errors"
	"sort"
	"strings"

	"github.com/dianabuilds/ardents/internal/core/app/services/dirquery"
	"github.com/dianabuilds/ardents/internal/core/app/services/servicedesc"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/capabilities"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/envelopev2"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func (r *Runtime) handleDirQueryV2(req tasks.Request, env *envelopev2.Envelope) ([][]byte, error) {
	input, err := dirquery.DecodeInput(req.Input)
	if err != nil {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}
	limit := input.Limit
	if limit == 0 {
		limit = 50
	}
	if limit > 50 {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}
	queryBytes, err := codec.Marshal(req.Input)
	if err != nil {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}
	queryHash := sha256.Sum256(queryBytes)
	nowMs := timeutil.NowUnixMs()

	results := r.buildDirQueryResults(input, nowMs)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].ServiceID < results[j].ServiceID
		}
		return results[i].Score > results[j].Score
	})
	if uint64(len(results)) > limit {
		results = results[:limit]
	}

	body := dirquery.ResultBody{
		V:           dirquery.Version,
		QueryHash:   queryHash[:],
		IssuedAtMs:  nowMs,
		ExpiresAtMs: nowMs + 60_000,
		Results:     results,
	}
	node := contentnode.Node{
		V:           1,
		Type:        dirquery.NodeType,
		CreatedAtMs: nowMs,
		Owner:       r.identity.ID,
		Links:       []contentnode.Link{},
		Body:        body,
		Policy: map[string]any{
			"v":          uint64(1),
			"visibility": "public",
		},
	}
	if r.identity.PrivateKey == nil || r.identity.ID == "" {
		return nil, errors.New("ERR_SIG_REQUIRED")
	}
	if err := contentnode.Sign(&node, r.identity.PrivateKey); err != nil {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}
	nodeBytes, nodeID, err := contentnode.EncodeWithCID(node)
	if err != nil {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}
	if err := contentnode.VerifyBytes(nodeBytes, nodeID); err != nil {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}
	if r.store != nil {
		_ = r.store.Put(nodeID, nodeBytes)
	}
	accept := r.buildTaskAcceptV2(req.TaskID, env)
	result := r.buildTaskResultV2(req.TaskID, nodeID, env)
	return append(accept, result...), nil
}

func (r *Runtime) buildDirQueryResults(input dirquery.Input, nowMs int64) []dirquery.ResultItem {
	if r.netdb == nil {
		return nil
	}
	heads := r.netdb.ServiceHeadsSnapshot(nowMs)
	if len(heads) == 0 {
		return nil
	}
	results := make([]dirquery.ResultItem, 0, len(heads))
	for _, head := range heads {
		descBytes, err := r.FetchNode(context.Background(), head.DescriptorCID)
		if err != nil {
			continue
		}
		if err := contentnode.VerifyBytes(descBytes, head.DescriptorCID); err != nil {
			continue
		}
		var node contentnode.Node
		if err := contentnode.Decode(descBytes, &node); err != nil {
			continue
		}
		body, err := servicedesc.ValidateV2(node)
		if err != nil {
			continue
		}
		if body.ServiceID != head.ServiceID || body.OwnerIdentityID != head.OwnerIdentityID || body.ServiceName != head.ServiceName {
			continue
		}
		if prefix := strings.TrimSpace(input.Query.ServiceNamePrefix); prefix != "" {
			if !strings.HasPrefix(body.ServiceName, prefix) {
				continue
			}
		}
		jobTypes := extractJobTypes(body.Capabilities)
		if !matchesRequires(jobTypes, input.Query.Requires) {
			continue
		}
		if mr := input.Query.MinResources; mr != nil {
			if !meetsResources(body.Resources, *mr) {
				continue
			}
		}
		digest, err := capabilities.Digest(jobTypes)
		if err != nil {
			continue
		}
		score := int64(0)
		if r.book.IsTrustedIdentity(body.OwnerIdentityID, nowMs) {
			score += 100
		}
		for _, req := range input.Query.Requires {
			if hasJobType(jobTypes, req) {
				score += 10
			}
		}
		if input.Query.MinResources != nil {
			score += int64(minUint64(100, body.Resources["cpu_cores"]))
			score += int64(minUint64(100, body.Resources["ram_mb"]/1024))
		}
		results = append(results, dirquery.ResultItem{
			ServiceID:          body.ServiceID,
			OwnerIdentityID:    body.OwnerIdentityID,
			ServiceName:        body.ServiceName,
			CapabilitiesDigest: digest,
			Score:              score,
		})
	}
	return results
}

func extractJobTypes(caps []servicedesc.Capability) []string {
	out := make([]string, 0, len(caps))
	seen := map[string]struct{}{}
	for _, c := range caps {
		if c.JobType == "" {
			continue
		}
		if _, ok := seen[c.JobType]; ok {
			continue
		}
		seen[c.JobType] = struct{}{}
		out = append(out, c.JobType)
	}
	sort.Strings(out)
	return out
}

func matchesRequires(jobTypes []string, requires []string) bool {
	if len(requires) == 0 {
		return true
	}
	for _, req := range requires {
		if !hasJobType(jobTypes, req) {
			return false
		}
	}
	return true
}

func hasJobType(jobTypes []string, target string) bool {
	for _, jt := range jobTypes {
		if jt == target {
			return true
		}
	}
	return false
}

func meetsResources(res map[string]uint64, min dirquery.Resources) bool {
	if res == nil {
		return false
	}
	if min.CpuCores > 0 && res["cpu_cores"] < min.CpuCores {
		return false
	}
	if min.RamMB > 0 && res["ram_mb"] < min.RamMB {
		return false
	}
	return true
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
