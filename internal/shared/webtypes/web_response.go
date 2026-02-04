package webtypes

// ResponseV1 is a canonical CBOR body for content node type "web.response.v1".
// Kept in internal/shared because it's used by multiple boundary tools (CLI, integrations).
type ResponseV1 struct {
	V       uint64            `cbor:"v"`
	TaskID  string            `cbor:"task_id"`
	Status  uint16            `cbor:"status"`
	Headers map[string]string `cbor:"headers,omitempty"`
	Body    []byte            `cbor:"body,omitempty"`
}
