// Package adsched 实现直投广告保量排期。
//
// 上层投放任务提供流量预测、合同与截至当前边界的实际累计投放，
// 本库在内存中生成剩余时段的全局最优排期计划：
// 先最小化所有合同最终欠量之和，再最小化总节奏偏差。
package adsched

// SlotsPerDay 一天固定的 30 分钟时段数。
const SlotsPerDay = 48

// Pacing 投放节奏类型。
type Pacing int

const (
	// Even 均匀节奏：累计目标 = Q*k/H 向下取整。
	Even Pacing = iota
	// FrontLoaded 前置节奏：累计目标 = Q*k*(2H-k)/H^2 向下取整。
	FrontLoaded
)

// Contract 一份直投合同。起止范围含 Start、不含 End。
type Contract struct {
	ID        string   // 合同标识
	Publisher string   // 发布方
	Sizes     []string // 可投放素材尺寸集合
	Q         int64    // 整数保证量
	Start     int      // 起始时段（含），0..47
	End       int      // 结束时段（不含），1..48
	Pacing    Pacing   // 投放节奏
}

// Slot 一个广告位，属于一个发布方并固定一个素材尺寸。
type Slot struct {
	ID        string
	Publisher string
	Size      string
}

// Input 排期输入，全部保存在内存中。
type Input struct {
	Now       int                // 当前时点（总在时段边界），0..48
	Slots     []Slot             // 广告位列表
	Capacity  map[string][]int64 // 广告位 ID -> 各时段预测容量（长度 48）
	Contracts []Contract         // 合同列表
	Delivered map[string]int64   // 合同 ID -> 截至当前边界的实际累计投放数
}

// Plan 排期结果：合同 ID -> 广告位 ID -> 时段 -> 整数曝光数。
type Plan struct {
	Alloc map[string]map[string]map[int]int64
}

// pacingTarget 计算合同第 k 个时段末（k 从 1 计）的累计目标，
// q 为保证量，h 为合同总长度（时段数）。全部按精确整数运算。
func pacingTarget(p Pacing, q, k, h int64) int64 {
	if k <= 0 {
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
