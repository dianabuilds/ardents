package runtime

import (
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/aichat"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
)

func (r *Runtime) buildAITranscript(taskID string, input aichat.Input) (string, error) {
	transcript := map[string]any{
		"v":       uint64(1),
		"task_id": taskID,
		"messages": func() []map[string]any {
			out := make([]map[string]any, 0, len(input.Messages))
			for _, m := range input.Messages {
				out = append(out, map[string]any{
					"role":    m.Role,
					"content": m.Content,
				})
			}
			return out
		}(),
		"safety": map[string]any{
			"v":      uint64(1),
			"labels": []string{},
		},
	}
	if input.Policy.Visibility == "encrypted" {
		node, err := contentnode.EncryptNode(r.identity.ID, r.identity.PrivateKey, "ai.chat.transcript.v1", []contentnode.Link{}, transcript, input.Policy.Recipients)
		if err != nil {
			return "", errors.New("ERR_AI_POLICY_INVALID")
		}
		nodeBytes, nodeID, err := contentnode.EncodeWithCID(node)
		if err != nil {
			return "", errors.New("ERR_AI_POLICY_INVALID")
		}
		if r.store != nil {
			if err := r.store.Put(nodeID, nodeBytes); err != nil {
				return "", err
			}
		}
		return nodeID, nil
	}
	n := contentnode.Node{
		V:           1,
		Type:        "ai.chat.transcript.v1",
		CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
		Owner:       r.identity.ID,
		Links:       []contentnode.Link{},
		Body:        transcript,
		Policy: map[string]any{
			"v":          uint64(1),
			"visibility": "public",
		},
	}
	if err := contentnode.Sign(&n, r.identity.PrivateKey); err != nil {
		return "", errors.New("ERR_AI_POLICY_INVALID")
	}
	nodeBytes, nodeID, err := contentnode.EncodeWithCID(n)
	if err != nil {
		return "", errors.New("ERR_AI_POLICY_INVALID")
	}
	if r.store != nil {
		if err := r.store.Put(nodeID, nodeBytes); err != nil {
			return "", err
		}
	}
	return nodeID, nil
}
