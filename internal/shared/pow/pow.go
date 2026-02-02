package pow

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

var (
	ErrPowRequired = errors.New("ERR_POW_REQUIRED")
	ErrPowInvalid  = errors.New("ERR_POW_INVALID")
)

type Stamp struct {
	V          uint64 `cbor:"v"`
	Difficulty uint64 `cbor:"difficulty"`
	Nonce      []byte `cbor:"nonce"`
	Subject    []byte `cbor:"subject"`
}

func Subject(msgID string, tsMs int64, peerID string) []byte {
	h := sha256.New()
	h.Write([]byte(msgID))
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(tsMs))
	h.Write(buf[:])
	h.Write([]byte(peerID))
	sum := h.Sum(nil)
	return sum
}

func Verify(stamp *Stamp) error {
	if stamp == nil || stamp.V != 1 || len(stamp.Nonce) != 16 || len(stamp.Subject) != 32 {
		return ErrPowInvalid
	}
	h := sha256.New()
	h.Write(stamp.Subject)
	h.Write(stamp.Nonce)
	sum := h.Sum(nil)
	if leadingZeroBits(sum) < int(stamp.Difficulty) {
		return ErrPowInvalid
	}
	return nil
}

func Generate(subject []byte, difficulty uint64) (*Stamp, error) {
	if len(subject) != 32 {
		return nil, ErrPowInvalid
	}
	stamp := &Stamp{
		V:          1,
		Difficulty: difficulty,
		Nonce:      make([]byte, 16),
		Subject:    subject,
	}
	for {
		if _, err := rand.Read(stamp.Nonce); err != nil {
			return nil, err
		}
		h := sha256.New()
		h.Write(stamp.Subject)
		h.Write(stamp.Nonce)
		sum := h.Sum(nil)
		if leadingZeroBits(sum) >= int(stamp.Difficulty) {
			return stamp, nil
		}
	}
}

func leadingZeroBits(b []byte) int {
	total := 0
	for _, v := range b {
		if v == 0 {
			total += 8
			continue
		}
		for i := 7; i >= 0; i-- {
			if (v>>i)&1 == 0 {
				total++
			} else {
				return total
			}
		}
	}
	return total
}
