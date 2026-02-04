package quic

import (
	"encoding/binary"
	"errors"
	"io"
	"math"

	"github.com/dianabuilds/ardents/internal/shared/conv"
)

var ErrFrameTooLarge = errors.New("ERR_FRAME_TOO_LARGE")

func writeFrame(w io.Writer, payload []byte) error {
	if len(payload) > math.MaxUint32 {
		return ErrFrameTooLarge
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], conv.ClampIntToUint32(len(payload)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readFrame(r io.Reader, maxBytes uint64) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if maxBytes > 0 && uint64(n) > maxBytes {
		return nil, io.ErrUnexpectedEOF
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
