package adsched

// deliveryReward 每单位未来投放的奖励权重。
// 必须大于任何增广路上节奏费用绝对值之和的上界（2*SlotsPerDay），
// 从而保证"先最小化欠量、再最小化节奏偏差"的严格先后。
const deliveryReward = 4 * SlotsPerDay

// Schedule 根据输入生成剩余时段的全局最优排期计划。
// 即使容量不足，也返回全局最优的欠量计划，而不是拒绝排期。
func Schedule(in Input) Plan {
	now := in.Now
	if now < 0 {
		now = 0
	}
	plan := Plan{Alloc: map[string]map[string]map[int]int64{}}
	if now >= SlotsPerDay || len(in.Contracts) == 0 || len(in.Slots) == 0 {
		return plan
	}

	// 节点编号：0=源，1=汇，其后依次为 (广告位,时段) 节点与合同链节点。
	stNode := map[[2]int]int{}
	next := 2
	for si := range in.Slots {
		caps := in.Capacity[in.Slots[si].ID]
		for t := now; t < SlotsPerDay; t++ {
			if t < len(caps) && caps[t] > 0 {
				stNode[[2]int{si, t}] = next
				next++
			}
		}
	}
	type chainInfo struct {
		contract *Contract
		times    []int // 剩余履约时段
		nodes    []int // 每个剩余时段一个链节点
		actual   int64 // 截至当前边界的实际累计投放
	}
	chains := []chainInfo{}
	for ci := range in.Contracts {
		c := &in.Contracts[ci]
		rest := c.Q - in.Delivered[c.ID]
		if rest <= 0 || c.End <= now || c.End <= c.Start {
			continue
		}
		start := c.Start
		if start < now {
			start = now
		}
		ch := chainInfo{contract: c, actual: in.Delivered[c.ID]}
		for t := start; t < c.End && t < SlotsPerDay; t++ {
			ch.times = append(ch.times, t)
			ch.nodes = append(ch.nodes, next)
			next++
		}
		if len(ch.times) > 0 {
			chains = append(chains, ch)
		}
	}

	g := newMCMF(next)
	const src, snk = 0, 1
	for si := range in.Slots {
		caps := in.Capacity[in.Slots[si].ID]
		for t := now; t < SlotsPerDay && t < len(caps); t++ {
			if node, ok := stNode[[2]int{si, t}]; ok {
				g.addEdge(src, node, caps[t], 0)
			}
		}
	}

	// allocEdge 记录 (广告位,时段)->合同链节点 的分配边，用于提取计划。
	type allocEdge struct {
		contractID, slotID string
		t                  int
		node, idx          int
	}
	allocs := []allocEdge{}
	compatible := func(c *Contract, s *Slot) bool {
		if c.Publisher != s.Publisher {
			return false
		}
		for _, sz := range c.Sizes {
			if sz == s.Size {
				return true
			}
		}
		return false
	}

	for _, ch := range chains {
		c := ch.contract
		m := len(ch.times)
		// 分配边：(广告位,时段) -> 合同对应链节点。
		for si := range in.Slots {
			if !compatible(c, &in.Slots[si]) {
				continue
			}
			for i, t := range ch.times {
				if node, ok := stNode[[2]int{si, t}]; ok {
					g.addEdge(node, ch.nodes[i], inf, 0)
					allocs = append(allocs, allocEdge{
						contractID: c.ID, slotID: in.Slots[si].ID,
						t: t, node: node, idx: len(g.graph[node]) - 1,
					})
				}
			}
		}
		// 链边：携带截至该时段末的累计未来投放，费用为节奏偏差（斜率 ±1 的两条并联边）。
		h := int64(c.End - c.Start)
		for j := 0; j+1 < m; j++ {
			k := int64(ch.times[j] - c.Start + 1)
			target := pacingTarget(c.Pacing, c.Q, k, h) - ch.actual
			if target > 0 {
				g.addEdge(ch.nodes[j], ch.nodes[j+1], target, -1)
			}
			g.addEdge(ch.nodes[j], ch.nodes[j+1], inf, 1)
		}
		// 末段出边：容量为剩余保证量，费用 = -1（末段节奏斜率）- deliveryReward（履约奖励）。
		rest := c.Q - ch.actual
		g.addEdge(ch.nodes[m-1], snk, rest, -1-deliveryReward)
	}

	g.minCostMaxFlow(src, snk)

	for _, a := range allocs {
		used := inf - g.graph[a.node][a.idx].cap
		if used <= 0 {
			continue
		}
		if plan.Alloc[a.contractID] == nil {
			plan.Alloc[a.contractID] = map[string]map[int]int64{}
		}
		if plan.Alloc[a.contractID][a.slotID] == nil {
			plan.Alloc[a.contractID][a.slotID] = map[int]int64{}
		}
		plan.Alloc[a.contractID][a.slotID][a.t] += used
	}
	return plan
}
