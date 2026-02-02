package uuidv7

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"time"
)

var uuidV7Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var ErrInvalidUUIDv7 = errors.New("invalid uuidv7")

func New() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return "", err
	}

	ts := uint64(time.Now().UTC().UnixNano() / int64(time.Millisecond))
	b[0] = byte(ts >> 40)
	b[1] = byte(ts >> 32)
	b[2] = byte(ts >> 24)
	b[3] = byte(ts >> 16)
	b[4] = byte(ts >> 8)
	b[5] = byte(ts)

	b[6] = 0x70 | (b[6] & 0x0F)
	b[8] = 0x80 | (b[8] & 0x3F)

	return format(b), nil
}

func Validate(id string) error {
	if !uuidV7Re.MatchString(id) {
		return ErrInvalidUUIDv7
	}
	return nil
}

func format(b [16]byte) string {
	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])
	return string(buf[:])
}
