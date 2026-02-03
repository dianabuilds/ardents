package codec

import (
	"errors"
	"fmt"
	"sync"

	"github.com/fxamacker/cbor/v2"
)

var (
	encMode  cbor.EncMode
	decMode  cbor.DecMode
	initErr  error
	initOnce sync.Once
)

var ErrCBORInit = errors.New("ERR_CBOR_INIT")

func initCodec() error {
	initOnce.Do(func() {
		var err error
		encMode, err = cbor.CanonicalEncOptions().EncMode()
		if err != nil {
			initErr = err
			return
		}
		decMode, err = cbor.DecOptions{
			DupMapKey: cbor.DupMapKeyEnforcedAPF,
		}.DecMode()
		if err != nil {
			initErr = err
			return
		}
	})
	return initErr
}

func Marshal(v any) ([]byte, error) {
	if err := initCodec(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCBORInit, err)
	}
	return encMode.Marshal(v)
}

func Unmarshal(data []byte, v any) error {
	if err := initCodec(); err != nil {
		return fmt.Errorf("%w: %v", ErrCBORInit, err)
	}
	return decMode.Unmarshal(data, v)
}
