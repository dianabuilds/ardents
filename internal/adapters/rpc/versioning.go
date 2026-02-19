package rpc

const (
	rpcAPICurrentVersion      = 1
	rpcAPIMinSupportedVersion = 1
	rpcAPIDefaultVersion      = 1
	rpcNotificationVersion    = 1
)

func validateRPCAPIVersion(v *int) *rpcError {
	if v == nil {
		return nil
	}
	if *v < rpcAPIMinSupportedVersion {
		return &rpcError{
			Code:    -32081,
			Message: "rpc api version is deprecated and no longer supported",
		}
	}
	if *v > rpcAPICurrentVersion {
		return &rpcError{
			Code:    -32080,
			Message: "rpc api version is not supported by this server",
		}
	}
	return nil
}

func rpcVersionInfo() map[string]any {
	return map[string]any{
		"current_version":       rpcAPICurrentVersion,
		"min_supported_version": rpcAPIMinSupportedVersion,
		"default_version":       rpcAPIDefaultVersion,
		"policy":                "major-only; requests below min are rejected; requests above current are rejected",
	}
}
