package capabilities

import (
	"crypto/sha256"
	"sort"

	"github.com/dianabuilds/ardents/internal/shared/codec"
)

func Digest(jobTypes []string) ([]byte, error) {
	unique := make(map[string]struct{}, len(jobTypes))
	for _, jt := range jobTypes {
		if jt == "" {
			continue
		}
		unique[jt] = struct{}{}
	}
	list := make([]string, 0, len(unique))
	for jt := range unique {
		list = append(list, jt)
	}
	sort.Strings(list)
	data, err := codec.Marshal(list)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	return sum[:], nil
}
