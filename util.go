package jsn_net

import "cmp"

func clip[T cmp.Ordered](val, min, max T) T {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
