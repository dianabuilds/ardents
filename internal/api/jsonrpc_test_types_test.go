package api

const maxRPCBodyBytes int64 = 1 << 20 // 1 MiB

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
