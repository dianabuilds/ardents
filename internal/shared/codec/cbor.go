package codec

import "github.com/fxamacker/cbor/v2"

var (
	encMode cbor.EncMode
	decMode cbor.DecMode
)

func init() {
	var err error
	encMode, err = cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		panic(err)
	}
	decMode, err = cbor.DecOptions{
		DupMapKey: cbor.DupMapKeyEnforcedAPF,
	}.DecMode()
	if err != nil {
		panic(err)
	}
}

func Marshal(v any) ([]byte, error) {
	return encMode.Marshal(v)
}

func Unmarshal(data []byte, v any) error {
	return decMode.Unmarshal(data, v)
}
