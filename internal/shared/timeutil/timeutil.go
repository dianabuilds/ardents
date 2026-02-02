package timeutil

import "time"

func NowUnixMs() int64 {
	return time.Now().UTC().UnixNano() / int64(time.Millisecond)
}
