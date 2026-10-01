package adsched

// 最小费用最大流：每轮用 SPFA 在残量网络中找最短增广路并增广。
// 支持负费用边（初始图为 DAG，不存在负费用环）。

type edge struct {
	to   int
	rev  int
	cap  int64
	cost int64
}

type flowGraph struct {
	g [][]edge
}

func newFlowGraph(n int) *flowGraph {
	return &flowGraph{g: make([][]edge, n)}
}

// addEdge 添加一条有向边，返回边在 from 邻接表中的下标。
func (fg *flowGraph) addEdge(from, to int, cap, cost int64) int {
	fwd := edge{to: to, rev: len(fg.g[to]), cap: cap, cost: cost}
	bwd := edge{to: from, rev: len(fg.g[from]), cap: 0, cost: -cost}
	fg.g[from] = append(fg.g[from], fwd)
	fg.g[to] = append(fg.g[to], bwd)
	return len(fg.g[from]) - 1
}

const inf64 = int64(1) << 62

// minCostMaxFlow 从 s 到 t 持续沿最短路增广直到无法增广，
// 返回最大流量与对应的最小费用。
// 不变式：每次增广后，当前流量值下的费用最小。
func (fg *flowGraph) minCostMaxFlow(s, t int) (flow, cost int64) {
	n := len(fg.g)
	dist := make([]int64, n)
	prevV := make([]int, n)
	prevE := make([]int, n)
	inQ := make([]bool, n)

	for {
		for i := range dist {
			dist[i] = inf64
		}
		dist[s] = 0
		queue := []int{s}
		inQ[s] = true
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			inQ[v] = false
			for i, e := range fg.g[v] {
				if e.cap > 0 && dist[v]+e.cost < dist[e.to] {
					dist[e.to] = dist[v] + e.cost
					prevV[e.to] = v
					prevE[e.to] = i
					if !inQ[e.to] {
						inQ[e.to] = true
						queue = append(queue, e.to)
					}
				}
			}
		}
		if dist[t] == inf64 {
			return flow, cost
		}
		d := inf64
		for v := t; v != s; v = prevV[v] {
			if c := fg.g[prevV[v]][prevE[v]].cap; c < d {
				d = c
			}
		}
		for v := t; v != s; v = prevV[v] {
			e := &fg.g[prevV[v]][prevE[v]]
			e.cap -= d
			fg.g[v][e.rev].cap += d
		}
		flow += d
		cost += d * dist[t]
	}
}
