// Package adsched 实现直投广告保量排期。
// 输入与计划均保存在内存中，由上层投放任务直接调用，不包含接口层。
package adsched

// PeriodsPerDay 一天固定的 30 分钟时段数。
const PeriodsPerDay = 48

// Pacing 投放节奏类型。
type Pacing int

const (
	// Even 均匀节奏。
	Even Pacing = iota
	// FrontLoaded 前置节奏。
	FrontLoaded
)

// Slot 广告位。属于一个发布方并固定一个素材尺寸。
type Slot struct {
	ID        string
	Publisher string
	Size      string
	// Forecast[t] 为绝对时段 t 的预测容量，长度须为 PeriodsPerDay。
	// t < Input.Now 的条目会被忽略。
	Forecast []int64
}

// Contract 直投合同。
type Contract struct {
	ID        string
	Publisher string
	Sizes     []string // 可投放素材尺寸集合
	Q         int64    // 整数保证量
	Start     int      // 起始时段（含），绝对时段下标
	End       int      // 结束时段（不含），绝对时段下标
	Pacing    Pacing
	Actual    int64 // 截至当前边界的实际累计投放数，不超过 Q
}

// Input 一次排期的全部输入。
type Input struct {
	Now       int // 当前时点所在的时段边界，[0, PeriodsPerDay]
	Slots     []Slot
	Contracts []Contract
}

// Allocation 计划中的一条分配：某合同在某广告位、某剩余时段的整数曝光数。
type Allocation struct {
	ContractID string
	SlotID     string
	Period     int
	Amount     int64
}

// Plan 排期结果。
type Plan struct {
	Allocations []Allocation
}

// Delivered 返回合同在本次计划中的未来分配总量。
func (p *Plan) Delivered(contractID string) int64 {
	var total int64
	for _, a := range p.Allocations {
		if a.ContractID == contractID {
			total += a.Amount
		}
	}
	return total
}

// Underdelivery 返回合同最终未完成保证量：Q - Actual - 未来分配。
func (p *Plan) Underdelivery(c Contract) int64 {
	return c.Q - c.Actual - p.Delivered(c.ID)
}
