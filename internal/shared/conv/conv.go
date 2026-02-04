package conv

const (
	maxInt   = int(^uint(0) >> 1)
	maxInt64 = int64(^uint64(0) >> 1)
	maxU16   = ^uint16(0)
	maxU32   = ^uint32(0)
)

func ClampUint64ToInt(v uint64) int {
	if v > uint64(maxInt) {
		return maxInt
	}
	return int(v)
}

func ClampUint64ToInt64(v uint64) int64 {
	if v > uint64(maxInt64) {
		return maxInt64
	}
	return int64(v)
}

func ClampInt64ToUint64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}

func ClampIntToUint64(v int) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}

func ClampIntToUint16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > int(maxU16) {
		return maxU16
	}
	return uint16(v)
}

func ClampIntToUint32(v int) uint32 {
	if v < 0 {
		return 0
	}
	if v > int(maxU32) {
		return maxU32
	}
	return uint32(v)
}
