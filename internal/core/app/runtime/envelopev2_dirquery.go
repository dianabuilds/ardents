package runtime

import (
	"context"
	"crypto/sha256"
	"errors"
	"sort"
	"strings"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/app/services/dirquery"
	"github.com/dianabuilds/ardents/internal/core/app/services/servicedesc"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/capabilities"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/conv"
	"github.com/dianabuilds/ardents/internal/shared/envelopev2"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func (r *Runtime) handleDirQueryV2(req tasks.Request, env *envelopev2.Envelope) ([][]byte, error) {
	nowMs := timeutil.NowUnixMs()
	if !r.allowDirQuery(env, nowMs) {
		return nil, errors.New("ERR_DIR_RATE_LIMITED")
	}
	input, err := dirquery.DecodeInput(req.Input)
	if err != nil {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}
	limit, err := normalizeDirQueryLimit(input.Limit)
	if err != nil {
		return nil, err
	}
	queryHash, err := hashDirQuery(req.Input)
	if err != nil {
		return nil, err
	}
	results := r.rankDirQueryResults(input, nowMs, limit)
	nodeID, err := r.storeDirQueryNode(queryHash, results, nowMs)
	if err != nil {
		return nil, err
	}
	accept := r.buildTaskAcceptV2(req.TaskID, env)
	result := r.buildTaskResultV2(req.TaskID, nodeID, env)
	if r.metrics != nil {
		r.metrics.IncTaskResult(req.JobType)
	}
	return append(accept, result...), nil
}

func (r *Runtime) allowDirQuery(env *envelopev2.Envelope, nowMs int64) bool {
	if r == nil || r.dirQueryLimiter == nil {
		return true
	}
	key := ""
	if env != nil {
		if env.From.IdentityID != "" {
			key = env.From.IdentityID
		} else if env.From.ServiceID != "" {
			key = env.From.ServiceID
		}
	}
	return r.dirQueryLimiter.Allow(key, nowMs)
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
		item, ok := r.dirQueryResultFromHead(head, input, nowMs)
		if !ok {
			continue
		}
		results = append(results, item)
	}
	return results
}

func (r *Runtime) dirQueryResultFromHead(head netdb.ServiceHead, input dirquery.Input, nowMs int64) (dirquery.ResultItem, bool) {
	body, ok := r.loadDirQueryDescriptor(head)
	if !ok {
		return dirquery.ResultItem{}, false
	}
	jobTypes := extractJobTypes(body.Capabilities)
	if !dirQueryMatches(body, jobTypes, input) {
		return dirquery.ResultItem{}, false
	}
	digest, err := capabilities.Digest(jobTypes)
	if err != nil {
		return dirquery.ResultItem{}, false
	}
	score := r.dirQueryScore(body, jobTypes, input, nowMs)
	return dirquery.ResultItem{
		ServiceID:          body.ServiceID,
		OwnerIdentityID:    body.OwnerIdentityID,
		ServiceName:        body.ServiceName,
		CapabilitiesDigest: digest,
		Score:              score,
	}, true
}

func (r *Runtime) loadDirQueryDescriptor(head netdb.ServiceHead) (servicedesc.Descriptor, bool) {
	descBytes, err := r.FetchNode(context.Background(), head.DescriptorCID)
	if err != nil {
		return servicedesc.Descriptor{}, false
	}
	if err := contentnode.VerifyBytes(descBytes, head.DescriptorCID); err != nil {
		return servicedesc.Descriptor{}, false
	}
	var node contentnode.Node
	if err := contentnode.Decode(descBytes, &node); err != nil {
		return servicedesc.Descriptor{}, false
	}
	body, err := servicedesc.ValidateV2(node)
	if err != nil {
		return servicedesc.Descriptor{}, false
	}
	if body.ServiceID != head.ServiceID || body.OwnerIdentityID != head.OwnerIdentityID || body.ServiceName != head.ServiceName {
		return servicedesc.Descriptor{}, false
	}
	return body, true
}

func dirQueryMatches(body servicedesc.Descriptor, jobTypes []string, input dirquery.Input) bool {
	if prefix := strings.TrimSpace(input.Query.ServiceNamePrefix); prefix != "" {
		if !strings.HasPrefix(body.ServiceName, prefix) {
			return false
		}
	}
	if !matchesRequires(jobTypes, input.Query.Requires) {
		return false
	}
	if mr := input.Query.MinResources; mr != nil {
		if !meetsResources(body.Resources, *mr) {
			return false
		}
	}
	return true
}

func (r *Runtime) dirQueryScore(body servicedesc.Descriptor, jobTypes []string, input dirquery.Input, nowMs int64) int64 {
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
		score += conv.ClampUint64ToInt64(minUint64(100, body.Resources["cpu_cores"]))
		score += conv.ClampUint64ToInt64(minUint64(100, body.Resources["ram_mb"]/1024))
	}
	return score
}

func normalizeDirQueryLimit(limit uint64) (uint64, error) {
	if limit == 0 {
		return 50, nil
	}
	if limit > 50 {
		return 0, errors.New("ERR_PAYLOAD_DECODE")
	}
	return limit, nil
}

func hashDirQuery(input any) ([]byte, error) {
	queryBytes, err := codec.Marshal(input)
	if err != nil {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}
	sum := sha256.Sum256(queryBytes)
	return sum[:], nil
}

func (r *Runtime) rankDirQueryResults(input dirquery.Input, nowMs int64, limit uint64) []dirquery.ResultItem {
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
	return results
}

func (r *Runtime) storeDirQueryNode(queryHash []byte, results []dirquery.ResultItem, nowMs int64) (string, error) {
	node, err := r.buildDirQueryNode(queryHash, results, nowMs)
	if err != nil {
		return "", err
	}
	nodeBytes, nodeID, err := contentnode.EncodeWithCID(node)
	if err != nil {
		return "", errors.New("ERR_PAYLOAD_DECODE")
	}
	if err := contentnode.VerifyBytes(nodeBytes, nodeID); err != nil {
		return "", errors.New("ERR_PAYLOAD_DECODE")
	}
	if r.store != nil {
		_ = r.store.Put(nodeID, nodeBytes)
	}
	return nodeID, nil
}

func (r *Runtime) buildDirQueryNode(queryHash []byte, results []dirquery.ResultItem, nowMs int64) (contentnode.Node, error) {
	body := dirquery.ResultBody{
		V:           dirquery.Version,
		QueryHash:   queryHash,
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
		return contentnode.Node{}, errors.New("ERR_SIG_REQUIRED")
	}
	if err := contentnode.Sign(&node, r.identity.PrivateKey); err != nil {
		return contentnode.Node{}, errors.New("ERR_PAYLOAD_DECODE")
	}
	return node, nil
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
