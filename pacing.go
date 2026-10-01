package adsched

// cumulativeTarget 累计节奏目标。
// q 为原始保证量，h 为合同原始长度（时段数），k 为从合同起点起已结束的时段数。
// 均匀：floor(q*k/h)；前置：floor(q*k*(2h-k)/h^2)。全部按精确整数运算。
func cumulativeTarget(q, h, k int64, p Pacing) int64 {
	if k <= 0 || h <= 0 {
		return 0
	}
	if k >= h {
		return q
	}
	if p == FrontLoaded {
		return q * k * (2*h - k) / (h * h)
	}
	return q * k / h
}
