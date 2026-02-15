package api

// Keep backward-compatible exported constructors referenced inside the package
// so IDE inspections don't mark them as unused.
var (
	_ = NewServer
	_ = NewServerWithAddr
	_ = NewServerWithOptions
	_ = NewServiceForDaemon
	_ = NewServiceForDaemonWithDataDir
)
