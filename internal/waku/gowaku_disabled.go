//go:build !real_waku

package waku

func newGoWakuBackend() goWakuBackend {
	return nil
}
