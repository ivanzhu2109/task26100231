package adsched

import "testing"

func forecast(caps map[int]int64) []int64 {
	f := make([]int64, PeriodsPerDay)
	for t, c := range caps {
		f[t] = c
	}
	return f
}

func allocAmount(p *Plan, contractID string, period int) int64 {
	var total int64
	for _, a := range p.Allocations {
		if a.ContractID == contractID && a.Period == period {
			total += a.Amount
		}
	}
	return total
}

func TestCumulativeTarget(t *testing.T) {
	// Q=5, H=4
	evenWant := []int64{0, 1, 2, 3, 5}
	frontWant := []int64{0, 2, 3, 4, 5}
	for k := int64(0); k <= 4; k++ {
		if got := cumulativeTarget(5, 4, k, Even); got != evenWant[k] {
			t.Errorf("均匀 k=%d: got %d, want %d", k, got, evenWant[k])
		}
		if got := cumulativeTarget(5, 4, k, FrontLoaded); got != frontWant[k] {
			t.Errorf("前置 k=%d: got %d, want %d", k, got, frontWant[k])
		}
	}
}

// 尺寸灵活合同与尺寸受限合同争用容量：可全部履约时不得产生可避免的欠量。
func TestFlexibleVsRestricted(t *testing.T) {
	in := Input{
		Now: 0,
		Slots: []Slot{
			{ID: "sA", Publisher: "pub", Size: "a", Forecast: forecast(map[int]int64{0: 5})},
			{ID: "sB", Publisher: "pub", Size: "b", Forecast: forecast(map[int]int64{0: 5})},
		},
		Contracts: []Contract{
			{ID: "flex", Publisher: "pub", Sizes: []string{"a", "b"}, Q: 5, Start: 0, End: 1, Pacing: Even},
			{ID: "rest", Publisher: "pub", Sizes: []string{"a"}, Q: 5, Start: 0, End: 1, Pacing: Even},
		},
	}
	plan, err := Schedule(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Delivered("flex"); got != 5 {
		t.Errorf("flex 履约 %d, want 5", got)
	}
	if got := plan.Delivered("rest"); got != 5 {
		t.Errorf("rest 履约 %d, want 5（存在可避免的欠量）", got)
	}
	// 受限合同只能用 sA。
	for _, a := range plan.Allocations {
		if a.ContractID == "rest" && a.SlotID != "sA" {
			t.Errorf("rest 被分配到不兼容广告位 %s", a.SlotID)
		}
	}
}

// 中途重排：Q=5, H=4，前两段实际为 0，后两段各有 5 次兼容容量。
// 均匀计划应依次分配 3 和 2。
func TestReplanEven(t *testing.T) {
	in := Input{
		Now: 2,
		Slots: []Slot{
			{ID: "s", Publisher: "pub", Size: "a", Forecast: forecast(map[int]int64{2: 5, 3: 5})},
		},
		Contracts: []Contract{
			{ID: "c", Publisher: "pub", Sizes: []string{"a"}, Q: 5, Start: 0, End: 4, Pacing: Even, Actual: 0},
		},
	}
	plan, err := Schedule(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := allocAmount(plan, "c", 2); got != 3 {
		t.Errorf("时段2 分配 %d, want 3", got)
	}
	if got := allocAmount(plan, "c", 3); got != 2 {
		t.Errorf("时段3 分配 %d, want 2", got)
	}
}

// 同一重排场景，前置计划应依次分配 4 和 1。
func TestReplanFrontLoaded(t *testing.T) {
	in := Input{
		Now: 2,
		Slots: []Slot{
			{ID: "s", Publisher: "pub", Size: "a", Forecast: forecast(map[int]int64{2: 5, 3: 5})},
		},
		Contracts: []Contract{
			{ID: "c", Publisher: "pub", Sizes: []string{"a"}, Q: 5, Start: 0, End: 4, Pacing: FrontLoaded, Actual: 0},
		},
	}
	plan, err := Schedule(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := allocAmount(plan, "c", 2); got != 4 {
		t.Errorf("时段2 分配 %d, want 4", got)
	}
	if got := allocAmount(plan, "c", 3); got != 1 {
		t.Errorf("时段3 分配 %d, want 1", got)
	}
}

// 容量不足时返回全局最优欠量计划，而不是拒绝排期。
func TestShortageReturnsOptimalPlan(t *testing.T) {
	in := Input{
		Now: 0,
		Slots: []Slot{
			{ID: "s", Publisher: "pub", Size: "a", Forecast: forecast(map[int]int64{0: 4, 1: 2})},
		},
		Contracts: []Contract{
			{ID: "c", Publisher: "pub", Sizes: []string{"a"}, Q: 10, Start: 0, End: 2, Pacing: Even},
		},
	}
	plan, err := Schedule(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Delivered("c"); got != 6 {
		t.Errorf("履约 %d, want 6（容量上限）", got)
	}
	if got := plan.Underdelivery(in.Contracts[0]); got != 4 {
		t.Errorf("欠量 %d, want 4", got)
	}
}

// 目标严格有序：总欠量最小优先于节奏偏差。
// Q=5, H=2 均匀，目标为 2、5；容量 p0=4, p1=1。
// 履约 5 的偏差为 |4-2|+0=2；履约 4 可得偏差 1。必须选择履约 5。
func TestUnderdeliveryBeforePacing(t *testing.T) {
	in := Input{
		Now: 0,
		Slots: []Slot{
			{ID: "s", Publisher: "pub", Size: "a", Forecast: forecast(map[int]int64{0: 4, 1: 1})},
		},
		Contracts: []Contract{
			{ID: "c", Publisher: "pub", Sizes: []string{"a"}, Q: 5, Start: 0, End: 2, Pacing: Even},
		},
	}
	plan, err := Schedule(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Delivered("c"); got != 5 {
		t.Errorf("履约 %d, want 5（欠量目标应优先于节奏偏差）", got)
	}
	if got := allocAmount(plan, "c", 0); got != 4 {
		t.Errorf("时段0 分配 %d, want 4", got)
	}
}

// 约束校验：发布方/尺寸匹配、时段范围、容量上限、累计不超过 Q。
func TestConstraints(t *testing.T) {
	in := Input{
		Now: 1,
		Slots: []Slot{
			{ID: "sA", Publisher: "pub1", Size: "a", Forecast: forecast(map[int]int64{0: 100, 1: 3, 2: 3, 3: 100})},
			{ID: "sB", Publisher: "pub2", Size: "a", Forecast: forecast(map[int]int64{1: 100, 2: 100})},
			{ID: "sC", Publisher: "pub1", Size: "b", Forecast: forecast(map[int]int64{1: 100, 2: 100})},
		},
		Contracts: []Contract{
			// 只能匹配 sA，且时段范围为 [1,3)。
			{ID: "c", Publisher: "pub1", Sizes: []string{"a"}, Q: 10, Start: 1, End: 3, Pacing: Even, Actual: 2},
		},
	}
	plan, err := Schedule(in)
	if err != nil {
		t.Fatal(err)
	}
	perSlotPeriod := map[[2]interface{}]int64{}
	for _, a := range plan.Allocations {
		if a.ContractID != "c" {
			t.Errorf("出现未知合同分配 %+v", a)
		}
		if a.SlotID != "sA" {
			t.Errorf("分配到发布方/尺寸不匹配的广告位 %s", a.SlotID)
		}
		if a.Period < 1 || a.Period >= 3 {
			t.Errorf("分配到合同范围外的时段 %d", a.Period)
		}
		perSlotPeriod[[2]interface{}{a.SlotID, a.Period}] += a.Amount
	}
	for key, v := range perSlotPeriod {
		cap := in.Slots[0].Forecast[key[1].(int)]
		if v > cap {
			t.Errorf("广告位 %v 时段 %v 分配 %d 超过容量 %d", key[0], key[1], v, cap)
		}
	}
	if got := plan.Delivered("c"); got+2 > 10 {
		t.Errorf("实际累计+未来分配 %d 超过 Q=10", got+2)
	}
	// sA 在 [1,3) 内容量共 6，应全部用满。
	if got := plan.Delivered("c"); got != 6 {
		t.Errorf("履约 %d, want 6", got)
	}
}

// 已结束合同不产生分配；Forecast 长度非法时报错。
func TestEdgeCases(t *testing.T) {
	in := Input{
		Now: 4,
		Slots: []Slot{
			{ID: "s", Publisher: "pub", Size: "a", Forecast: forecast(map[int]int64{4: 10})},
		},
		Contracts: []Contract{
			{ID: "done", Publisher: "pub", Sizes: []string{"a"}, Q: 5, Start: 0, End: 4, Pacing: Even, Actual: 3},
		},
	}
	plan, err := Schedule(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Allocations) != 0 {
		t.Errorf("已结束合同不应有分配: %+v", plan.Allocations)
	}
	if got := plan.Underdelivery(in.Contracts[0]); got != 2 {
		t.Errorf("已结束合同欠量 %d, want 2", got)
	}

	bad := Input{
		Now:   0,
		Slots: []Slot{{ID: "s", Publisher: "pub", Size: "a", Forecast: make([]int64, 10)}},
	}
	if _, err := Schedule(bad); err == nil {
		t.Error("Forecast 长度非法应返回错误")
	}
}
