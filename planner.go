package adsched

import "fmt"

// Schedule 根据流量预测生成剩余时段计划。
// 优化目标严格有序：先最小化所有合同最终未完成保证量之和，
// 在该最优值下再最小化总节奏偏差。即使容量不足也返回全局最优欠量计划。
//
// 建模为最小费用最大流：
//   - 源 -> (广告位,时段)：容量为预测容量；
//   - (广告位,时段) -> 合同链节点：发布方与尺寸匹配且时段在合同范围内；
//   - 合同链边携带累计投放量，费用为对该时段末累计目标的分段线性偏差
//     （目标内每单位 -1，超出后每单位 +1，常数项不影响最优解）；
//   - 合同链尾 -> 汇：容量为剩余保证量 Q-Actual。
//
// 最大流对应最小总欠量；在最大流中费用最小对应总节奏偏差最小。
func Schedule(in Input) (*Plan, error) {
	if err := validate(in); err != nil {
		return nil, err
	}

	// 节点编号：0=源，1=汇，其后依次为 (广告位,时段) 节点与各合同链节点。
	const (
		src = 0
		snk = 1
	)
	next := 2
	type slotPeriodNode struct {
		slot   int
		period int
		node   int
	}
	var spNodes []slotPeriodNode
	for i, s := range in.Slots {
		for t := in.Now; t < PeriodsPerDay; t++ {
			if s.Forecast[t] > 0 {
				spNodes = append(spNodes, slotPeriodNode{slot: i, period: t, node: next})
				next++
			}
		}
	}
	type contractNodes struct {
		start int // 链节点起始编号，共 remain+1 个
		t0    int // 首个剩余时段（绝对下标）
		rem   int // 剩余时段数
	}
	cNodes := make([]contractNodes, len(in.Contracts))
	active := make([]bool, len(in.Contracts))
	for ci, c := range in.Contracts {
		t0 := max(c.Start, in.Now)
		rem := c.End - t0
		qRem := c.Q - c.Actual
		if rem <= 0 || qRem <= 0 {
			continue
		}
		active[ci] = true
		cNodes[ci] = contractNodes{start: next, t0: t0, rem: rem}
		next += rem + 1
	}

	fg := newFlowGraph(next)

	// 源 -> (广告位,时段)
	for _, sp := range spNodes {
		fg.addEdge(src, sp.node, in.Slots[sp.slot].Forecast[sp.period], 0)
	}

	// 记录 (广告位,时段) -> 合同链 的边，用于回读计划。
	type arcRef struct {
		from     int
		idx      int
		contract int
		slot     int
		period   int
	}
	var arcs []arcRef

	for ci, c := range in.Contracts {
		if !active[ci] {
			continue
		}
		cn := cNodes[ci]
		qRem := c.Q - c.Actual
		h := int64(c.End - c.Start)

		// 链边 p_{j-1} -> p_j 携带第 j 个剩余时段末的累计投放量，
		// 费用为 |Actual + X - 累计目标| 的分段线性边际（去掉常数项）。
		for j := 1; j <= cn.rem; j++ {
			k := int64(cn.t0 + j - c.Start)
			target := cumulativeTarget(c.Q, h, k, c.Pacing)
			m := target - c.Actual
			if m < 0 {
				m = 0
			}
			if m > qRem {
				m = qRem
			}
			from := cn.start + j - 1
			to := cn.start + j
			if m > 0 {
				fg.addEdge(from, to, m, -1)
			}
			if qRem-m > 0 {
				fg.addEdge(from, to, qRem-m, 1)
			}
		}
		// 链尾 -> 汇：限制未来分配总量不超过剩余保证量。
		fg.addEdge(cn.start+cn.rem, snk, qRem, 0)

		// (广告位,时段) -> 链节点 p_{t-t0}
		sizes := make(map[string]bool, len(c.Sizes))
		for _, sz := range c.Sizes {
			sizes[sz] = true
		}
		for _, sp := range spNodes {
			s := in.Slots[sp.slot]
			if s.Publisher != c.Publisher || !sizes[s.Size] {
				continue
			}
			if sp.period < cn.t0 || sp.period >= c.End {
				continue
			}
			to := cn.start + (sp.period - cn.t0)
			idx := fg.addEdge(sp.node, to, qRem, 0)
			arcs = append(arcs, arcRef{from: sp.node, idx: idx, contract: ci, slot: sp.slot, period: sp.period})
		}
	}

	fg.minCostMaxFlow(src, snk)

	plan := &Plan{}
	for _, a := range arcs {
		e := fg.g[a.from][a.idx]
		// 反向边残量即该边上的流量。
		if flow := fg.g[e.to][e.rev].cap; flow > 0 {
			plan.Allocations = append(plan.Allocations, Allocation{
				ContractID: in.Contracts[a.contract].ID,
				SlotID:     in.Slots[a.slot].ID,
				Period:     a.period,
				Amount:     flow,
			})
		}
	}
	return plan, nil
}

func validate(in Input) error {
	if in.Now < 0 || in.Now > PeriodsPerDay {
		return fmt.Errorf("adsched: Now=%d 超出 [0,%d]", in.Now, PeriodsPerDay)
	}
	for i, s := range in.Slots {
		if len(s.Forecast) != PeriodsPerDay {
			return fmt.Errorf("adsched: 广告位 %d(%s) 的 Forecast 长度为 %d，应为 %d",
				i, s.ID, len(s.Forecast), PeriodsPerDay)
		}
	}
	for i, c := range in.Contracts {
		if c.Start < 0 || c.End > PeriodsPerDay || c.Start >= c.End {
			return fmt.Errorf("adsched: 合同 %d(%s) 起止时段 [%d,%d) 非法", i, c.ID, c.Start, c.End)
		}
		if c.Q < 0 {
			return fmt.Errorf("adsched: 合同 %d(%s) 保证量为负", i, c.ID)
		}
		if c.Actual < 0 || c.Actual > c.Q {
			return fmt.Errorf("adsched: 合同 %d(%s) 实际累计 %d 不在 [0,Q]", i, c.ID, c.Actual)
		}
	}
	return nil
}
