package adsched

import "container/heap"

const inf = int64(1) << 60

// mcmf 是支持负费用（无负环）的最小费用流，
// 采用 SSP：Bellman-Ford 初始化势，Dijkstra + 势不断增广最短增广路。
type mcmf struct {
	n     int
	graph [][]edge
}

type edge struct {
	to, rev   int
	cap, cost int64
}

func newMCMF(n int) *mcmf {
	return &mcmf{n: n, graph: make([][]edge, n)}
}

// addEdge 添加容量为 cap、单位费用为 cost 的有向边。
func (g *mcmf) addEdge(u, v int, cap, cost int64) {
	if cap <= 0 {
		return
	}
	g.graph[u] = append(g.graph[u], edge{to: v, rev: len(g.graph[v]), cap: cap, cost: cost})
	g.graph[v] = append(g.graph[v], edge{to: u, rev: len(g.graph[u]) - 1, cap: 0, cost: -cost})
}

// minCostMaxFlow 不断沿最短增广路增广直到没有增广路，返回流量与费用。
func (g *mcmf) minCostMaxFlow(s, t int) (flow, cost int64) {
	pot := g.bellmanFord(s)
	for {
		dist, prevV, prevE := g.dijkstra(s, pot)
		if dist[t] == inf {
			return flow, cost
		}
		for v := 0; v < g.n; v++ {
			if dist[v] < inf {
				pot[v] += dist[v]
			}
		}
		d := inf
		for v := t; v != s; v = prevV[v] {
			if c := g.graph[prevV[v]][prevE[v]].cap; c < d {
				d = c
			}
		}
		for v := t; v != s; v = prevV[v] {
			e := &g.graph[prevV[v]][prevE[v]]
			e.cap -= d
			g.graph[v][e.rev].cap += d
		}
		flow += d
		cost += d * pot[t]
	}
}

// bellmanFord 计算初始势，使所有边的约化费用非负。
func (g *mcmf) bellmanFord(s int) []int64 {
	dist := make([]int64, g.n)
	for i := range dist {
		dist[i] = inf
	}
	dist[s] = 0
	for iter := 0; iter < g.n; iter++ {
		updated := false
		for v := 0; v < g.n; v++ {
			if dist[v] == inf {
				continue
			}
			for _, e := range g.graph[v] {
				if e.cap > 0 && dist[v]+e.cost < dist[e.to] {
					dist[e.to] = dist[v] + e.cost
					updated = true
				}
			}
		}
		if !updated {
			break
		}
	}
	for i := range dist {
		if dist[i] == inf {
			dist[i] = 0
		}
	}
	return dist
}

// dijkstra 在约化费用上求最短路，返回距离与父边。
func (g *mcmf) dijkstra(s int, pot []int64) (dist []int64, prevV, prevE []int) {
	dist = make([]int64, g.n)
	prevV = make([]int, g.n)
	prevE = make([]int, g.n)
	for i := range dist {
		dist[i] = inf
	}
	dist[s] = 0
	pq := &pqueue{{v: s, d: 0}}
	for pq.Len() > 0 {
		cur := heap.Pop(pq).(item)
		if cur.d > dist[cur.v] {
			continue
		}
		for i, e := range g.graph[cur.v] {
			if e.cap <= 0 {
				continue
			}
			nd := cur.d + e.cost + pot[cur.v] - pot[e.to]
			if nd < dist[e.to] {
				dist[e.to] = nd
				prevV[e.to] = cur.v
				prevE[e.to] = i
				heap.Push(pq, item{v: e.to, d: nd})
			}
		}
	}
	return dist, prevV, prevE
}

type item struct {
	v int
	d int64
}

type pqueue []item

func (q pqueue) Len() int            { return len(q) }
func (q pqueue) Less(i, j int) bool  { return q[i].d < q[j].d }
func (q pqueue) Swap(i, j int)       { q[i], q[j] = q[j], q[i] }
func (q *pqueue) Push(x interface{}) { *q = append(*q, x.(item)) }
func (q *pqueue) Pop() interface{} {
	old := *q
	n := len(old)
	it := old[n-1]
	*q = old[:n-1]
	return it
}
