package rpckit

// Error is a transport-level RPC error that can be mapped by the caller
// to a concrete wire format (e.g. JSON-RPC error object).
type Error struct {
	Code    int
	Message string
}

func InvalidParams() *Error {
	return &Error{Code: -32602, Message: "invalid params"}
}

func ServiceError(code int, err error) *Error {
	return &Error{Code: code, Message: err.Error()}
}
