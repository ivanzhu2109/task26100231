package adsched

import "testing"

// validatePlan 校验计划满足全部硬约束。
func validatePlan(t *testing.T, in Input, plan Plan) {
	t.Helper()
	slots := map[string]Slot{}
	for _, s := range in.Slots {
		slots[s.ID] = s
	}
	contracts := map[string]Contract{}
	for _, c := range in.Contracts {
		contracts[c.ID] = c
	}
	used := map[[2]interface{}]int64{} // (slotID, t) -> 总分配
	for cid, bySlot := range plan.Alloc {
		c, ok := contracts[cid]
		if !ok {
			t.Fatalf("计划包含未知合同 %s", cid)
		}
		var total int64
		for sid, byTime := range bySlot {
			s, ok := slots[sid]
			if !ok {
				t.Fatalf("合同 %s 分配到未知广告位 %s", cid, sid)
			}
			if s.Publisher != c.Publisher {
				t.Fatalf("合同 %s 分配到发布方不匹配的广告位 %s", cid, sid)
			}
			match := false
			for _, sz := range c.Sizes {
				if sz == s.Size {
					match = true
				}
			}
			if !match {
				t.Fatalf("合同 %s 分配到尺寸不匹配的广告位 %s", cid, sid)
			}
			for ts, v := range byTime {
				if v <= 0 {
					t.Fatalf("合同 %s 在 %s 时段 %d 的分配非正: %d", cid, sid, ts, v)
				}
				if ts < in.Now || ts >= SlotsPerDay || ts < c.Start || ts >= c.End {
					t.Fatalf("合同 %s 分配到范围外时段 %d", cid, ts)
				}
				used[[2]interface{}{sid, ts}] += v
				total += v
			}
		}
		if got := in.Delivered[cid] + total; got > c.Q {
			t.Fatalf("合同 %s 实际累计+未来分配=%d 超过保证量 %d", cid, got, c.Q)
		}
	}
	for key, v := range used {
		sid, ts := key[0].(string), key[1].(int)
		caps := in.Capacity[sid]
		var capv int64
		if ts < len(caps) {
			capv = caps[ts]
		}
		if v > capv {
			t.Fatalf("广告位 %s 时段 %d 分配 %d 超过预测容量 %d", sid, ts, v, capv)
		}
	}
}

func totalAlloc(plan Plan, cid string) int64 {
	var sum int64
	for _, byTime := range plan.Alloc[cid] {
		for _, v := range byTime {
			sum += v
		}
	}
	return sum
}

func allocAt(plan Plan, cid, sid string, ts int) int64 {
	return plan.Alloc[cid][sid][ts]
}

func capsOf(n int, vals ...int64) []int64 {
	caps := make([]int64, SlotsPerDay)
	for i, v := range vals {
		if i < n {
			caps[i] = v
		}
	}
	return caps
}

// TestFlexVsRestricted 尺寸灵活合同与尺寸受限合同争用容量，
// 可全部履约时不得产生可避免的欠量。
func TestFlexVsRestricted(t *testing.T) {
	in := Input{
		Now: 0,
		Slots: []Slot{
			{ID: "big", Publisher: "pub", Size: "big"},
			{ID: "small", Publisher: "pub", Size: "small"},
		},
		Capacity: map[string][]int64{
			"big":   capsOf(2, 5, 5),
			"small": capsOf(2, 5, 5),
		},
		Contracts: []Contract{
			{ID: "flex", Publisher: "pub", Sizes: []string{"big", "small"}, Q: 10, Start: 0, End: 2, Pacing: Even},
			{ID: "restricted", Publisher: "pub", Sizes: []string{"small"}, Q: 10, Start: 0, End: 2, Pacing: Even},
		},
		Delivered: map[string]int64{},
	}
	plan := Schedule(in)
	validatePlan(t, in, plan)
	if got := totalAlloc(plan, "flex"); got != 10 {
		t.Fatalf("flex 应全部履约 10，实际 %d", got)
	}
	if got := totalAlloc(plan, "restricted"); got != 10 {
		t.Fatalf("restricted 应全部履约 10，实际 %d", got)
	}
	// small 总容量仅 10，restricted 必须独占 small，flex 只能用 big。
	if got := totalAlloc(plan, "restricted"); got != 10 {
		t.Fatalf("restricted 欠量: %d", got)
	}
	if v := allocAt(plan, "flex", "small", 0) + allocAt(plan, "flex", "small", 1); v != 0 {
		t.Fatalf("flex 不应占用 small（否则 restricted 必欠量），实际占用 %d", v)
	}
}

// TestReplanEven 中途重排：Q=5、H=4、前两段实际为 0，
// 后两段各至少 5 次兼容容量且无竞争时，均匀计划应依次分配 3 和 2。
func TestReplanEven(t *testing.T) {
	in := Input{
		Now:   2,
		Slots: []Slot{{ID: "s", Publisher: "pub", Size: "m"}},
		Capacity: map[string][]int64{
			"s": capsOf(4, 0, 0, 5, 5),
		},
		Contracts: []Contract{
			{ID: "c", Publisher: "pub", Sizes: []string{"m"}, Q: 5, Start: 0, End: 4, Pacing: Even},
		},
		Delivered: map[string]int64{"c": 0},
	}
	plan := Schedule(in)
	validatePlan(t, in, plan)
	if got := allocAt(plan, "c", "s", 2); got != 3 {
		t.Fatalf("均匀节奏时段 2 应分配 3，实际 %d", got)
	}
	if got := allocAt(plan, "c", "s", 3); got != 2 {
		t.Fatalf("均匀节奏时段 3 应分配 2，实际 %d", got)
	}
}

// TestReplanFrontLoaded 同上的中途重排，前置计划应依次分配 4 和 1。
func TestReplanFrontLoaded(t *testing.T) {
	in := Input{
		Now:   2,
		Slots: []Slot{{ID: "s", Publisher: "pub", Size: "m"}},
		Capacity: map[string][]int64{
			"s": capsOf(4, 0, 0, 5, 5),
		},
		Contracts: []Contract{
			{ID: "c", Publisher: "pub", Sizes: []string{"m"}, Q: 5, Start: 0, End: 4, Pacing: FrontLoaded},
		},
		Delivered: map[string]int64{"c": 0},
	}
	plan := Schedule(in)
	validatePlan(t, in, plan)
	if got := allocAt(plan, "c", "s", 2); got != 4 {
		t.Fatalf("前置节奏时段 2 应分配 4，实际 %d", got)
	}
	if got := allocAt(plan, "c", "s", 3); got != 1 {
		t.Fatalf("前置节奏时段 3 应分配 1，实际 %d", got)
	}
}

// TestUnderdeliveryBestEffort 容量不足时返回全局最优欠量计划而非拒绝。
func TestUnderdeliveryBestEffort(t *testing.T) {
	in := Input{
		Now:   0,
		Slots: []Slot{{ID: "s", Publisher: "pub", Size: "m"}},
		Capacity: map[string][]int64{
			"s": capsOf(2, 2, 1),
		},
		Contracts: []Contract{
			{ID: "c", Publisher: "pub", Sizes: []string{"m"}, Q: 5, Start: 0, End: 2, Pacing: Even},
		},
		Delivered: map[string]int64{},
	}
	plan := Schedule(in)
	validatePlan(t, in, plan)
	if got := totalAlloc(plan, "c"); got != 3 {
		t.Fatalf("容量不足时应尽量履约 3，实际 %d", got)
	}
}

// TestDeliveryBeatsDeviation 履约目标严格优先于节奏偏差：
// 提前投放会加大节奏偏差，但欠量更小，必须选择投放。
func TestDeliveryBeatsDeviation(t *testing.T) {
	in := Input{
		Now:   0,
		Slots: []Slot{{ID: "s", Publisher: "pub", Size: "m"}},
		Capacity: map[string][]int64{
			"s": capsOf(3, 1, 0, 0),
		},
		Contracts: []Contract{
			{ID: "c", Publisher: "pub", Sizes: []string{"m"}, Q: 1, Start: 0, End: 3, Pacing: Even},
		},
		Delivered: map[string]int64{},
	}
	plan := Schedule(in)
	validatePlan(t, in, plan)
	if got := allocAt(plan, "c", "s", 0); got != 1 {
		t.Fatalf("应优先履约（时段 0 分配 1），实际 %d", got)
	}
}

// TestActualCountsTowardQ 过去投放不可改写，实际累计计入保证量。
func TestActualCountsTowardQ(t *testing.T) {
	in := Input{
		Now:   1,
		Slots: []Slot{{ID: "s", Publisher: "pub", Size: "m"}},
		Capacity: map[string][]int64{
			"s": capsOf(3, 0, 10, 10),
		},
		Contracts: []Contract{
			{ID: "c", Publisher: "pub", Sizes: []string{"m"}, Q: 5, Start: 0, End: 3, Pacing: Even},
		},
		Delivered: map[string]int64{"c": 4},
	}
	plan := Schedule(in)
	validatePlan(t, in, plan)
	if got := totalAlloc(plan, "c"); got != 1 {
		t.Fatalf("实际已投 4、Q=5，未来只应分配 1，实际 %d", got)
	}
}

// TestMismatchNoAlloc 发布方或尺寸不匹配不得分配。
func TestMismatchNoAlloc(t *testing.T) {
	in := Input{
		Now: 0,
		Slots: []Slot{
			{ID: "s1", Publisher: "other", Size: "m"},
			{ID: "s2", Publisher: "pub", Size: "xl"},
		},
		Capacity: map[string][]int64{
			"s1": capsOf(1, 10),
			"s2": capsOf(1, 10),
		},
		Contracts: []Contract{
			{ID: "c", Publisher: "pub", Sizes: []string{"m"}, Q: 5, Start: 0, End: 1, Pacing: Even},
		},
		Delivered: map[string]int64{},
	}
	plan := Schedule(in)
	validatePlan(t, in, plan)
	if got := totalAlloc(plan, "c"); got != 0 {
		t.Fatalf("无兼容广告位时不应分配，实际 %d", got)
	}
}

// TestCompetitionGlobalOptimum 多合同争用同一批广告位时全局最优：
// 受限合同独占稀缺流量，灵活合同用其余流量，双方均不欠量。
func TestCompetitionGlobalOptimum(t *testing.T) {
	in := Input{
		Now: 0,
		Slots: []Slot{
			{ID: "a", Publisher: "pub", Size: "banner"},
			{ID: "b", Publisher: "pub", Size: "video"},
		},
		Capacity: map[string][]int64{
			"a": capsOf(1, 7),
			"b": capsOf(1, 3),
		},
		Contracts: []Contract{
			{ID: "flex", Publisher: "pub", Sizes: []string{"banner", "video"}, Q: 6, Start: 0, End: 1, Pacing: Even},
			{ID: "bannerOnly", Publisher: "pub", Sizes: []string{"banner"}, Q: 4, Start: 0, End: 1, Pacing: Even},
		},
		Delivered: map[string]int64{},
	}
	plan := Schedule(in)
	validatePlan(t, in, plan)
	if got := totalAlloc(plan, "flex"); got != 6 {
		t.Fatalf("flex 应履约 6，实际 %d", got)
	}
	if got := totalAlloc(plan, "bannerOnly"); got != 4 {
		t.Fatalf("bannerOnly 应履约 4，实际 %d", got)
	}
}

// TestPacingTarget 累计目标公式的精确整数语义。
func TestPacingTarget(t *testing.T) {
	cases := []struct {
		p             Pacing
		q, k, h, want int64
	}{
		{Even, 5, 1, 4, 1},
		{Even, 5, 2, 4, 2},
		{Even, 5, 3, 4, 3},
		{Even, 5, 4, 4, 5},
		{FrontLoaded, 5, 1, 4, 2}, // 5*1*7/16 = 35/16，向下取整为 2
		{FrontLoaded, 5, 2, 4, 3}, // 5*2*6/16 = 60/16 = 3
		{FrontLoaded, 5, 3, 4, 4}, // 5*3*5/16 = 75/16 = 4
		{FrontLoaded, 5, 4, 4, 5}, // 5*4*4/16 = 5
		{Even, 10, 0, 4, 0},
	}
	for _, tc := range cases {
		if got := pacingTarget(tc.p, tc.q, tc.k, tc.h); got != tc.want {
			t.Fatalf("pacingTarget(%v, %d, %d, %d) = %d, 期望 %d", tc.p, tc.q, tc.k, tc.h, got, tc.want)
		}
	}
}
