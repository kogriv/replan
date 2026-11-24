package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kogriv/replan/internal/domain"
)

// SharedCapacityOverload describes a set of pools whose eligible members
// cannot supply their pooled load after accounting for direct load.
type SharedCapacityOverload struct {
	PoolIDs        []string `json:"pool_ids"`
	MemberIDs      []string `json:"member_ids"`
	Unit           string   `json:"unit"`
	Demand         float64  `json:"demand"`
	UsableCapacity float64  `json:"usable_capacity"`
	Shortfall      float64  `json:"shortfall"`
}

type sharedPoolLoad struct {
	id      string
	load    float64
	members []string
}

// Filtered views still need demand from every competing pool and member in
// the selected unit. Retain only plausible participants as hidden rows.
func (c *workloadCalculation) filteredSharedTargets() map[string]bool {
	if len(c.pools) == 0 || (c.req.ResourceID == "" && c.req.ResourceClass == "") {
		return nil
	}
	targets := make(map[string]bool)
	for _, demand := range c.workspace.ResourceDemands {
		if demand.Unit == "concurrent_slots" || (c.req.Unit != "" && demand.Unit != c.req.Unit) ||
			!domain.DemandAddsLevelledLoad(demand) || (demand.ReservedAmount == 0 && demand.Remaining() == 0) {
			continue
		}
		id, _ := demandTargetID(demand)
		pool, ok := c.pools[id]
		if !ok || pool.Unresolved || len(pool.Members) == 0 {
			continue
		}
		if c.hasWeekFilter {
			span := demandWindow(demand, c.tasks)
			if !span.dated || !span.intersects(c.weekStart, c.weekEnd) {
				continue
			}
		}
		targets[id+"\x00"+demand.Unit] = true
		for _, member := range pool.Members {
			targets[member.ID+"\x00"+demand.Unit] = true
		}
	}
	return targets
}

func (c *workloadCalculation) addSharedCapacityWarnings() {
	if len(c.pools) == 0 {
		return
	}
	candidates := make(map[string][]string)
	for key, row := range c.rows {
		if pool, ok := c.pools[row.ResourceID]; ok && !pool.Unresolved &&
			len(pool.Members) != 0 && row.Unit != "concurrent_slots" && workloadRowLoad(row) > 0 {
			candidates[row.Unit] = append(candidates[row.Unit], key)
		}
	}
	if len(candidates) == 0 {
		return
	}

	scopes := c.sharedCapacityScopes(candidates)

	units := make([]string, 0, len(candidates))
	for unit := range candidates {
		units = append(units, unit)
	}
	slices.Sort(units)
	for _, unit := range units {
		keys := candidates[unit]
		slices.Sort(keys)
		var pools []sharedPoolLoad
		residual := make(map[string]float64)
		for _, key := range keys {
			row := c.rows[key]
			if scopes[key].unsupported || c.capacityByResourceUnit[key] > 0 {
				continue
			}
			members := make(map[string]bool)
			var period string
			for _, ref := range c.pools[row.ResourceID].Members {
				memberKey := ref.ID + "\x00" + unit
				scope := scopes[memberKey]
				if _, exists := c.resources[ref.ID]; !exists || !scope.known || scope.unsupported ||
					(len(members) != 0 && scope.period != period) {
					members = nil
					break
				}
				members[ref.ID] = true
				period = scope.period
			}
			if len(members) == 0 {
				continue
			}
			memberIDs := sortedMapKeys(members)
			pools = append(pools, sharedPoolLoad{row.ResourceID, workloadRowLoad(row), memberIDs})
			for _, id := range memberIDs {
				memberKey := id + "\x00" + unit
				if _, seen := residual[id]; !seen {
					residual[id] = max(0, roundDashboardFloat(c.capacityByResourceUnit[memberKey])-workloadRowLoad(c.rows[memberKey]))
				}
			}
		}
		if len(pools) != 0 {
			c.checkSharedCapacity(unit, pools, residual)
		}
	}
}

type capacityScope struct {
	period      string
	known       bool
	unsupported bool
}

func (c *workloadCalculation) sharedCapacityScopes(candidates map[string][]string) map[string]capacityScope {
	// Track comparison periods separately from row totals. Whole-plan rows
	// can combine capacity records with different periods.
	scopes := make(map[string]capacityScope)
	for _, capacity := range c.workspace.Capacities {
		if len(candidates[capacity.Unit]) == 0 {
			continue
		}
		key := capacity.ResourceRef.ID + "\x00" + capacity.Unit
		scope := scopes[key]
		// The row peak rule recognizes concurrent periods even outside
		// the effective dates. Other unsupported supply must be in scope.
		applies := c.capacityApplies(capacity)
		window := capacity.Period == "instant" || capacity.Type == "concurrent_capacity" || capacity.Type == "concurrent"
		if capacity.Period == "concurrent" || (applies && window) {
			scope.unsupported = true
		} else if applies && (!c.hasWeekFilter || capacity.Period != "month") {
			period := capacity.Period
			if c.hasWeekFilter {
				period = "week"
			}
			scope.unsupported = scope.unsupported || (scope.known && scope.period != period)
			scope.period, scope.known = period, true
		}
		scopes[key] = scope
	}

	return scopes
}

func (c *workloadCalculation) checkSharedCapacity(unit string, pools []sharedPoolLoad, residual map[string]float64) {
	const source, sink = 0, 1
	graph := make(capacityFlow, 2+len(pools)+len(residual))
	memberIDs := make([]string, 0, len(residual))
	for id := range residual {
		memberIDs = append(memberIDs, id)
	}
	slices.Sort(memberIDs)
	memberNodes := make(map[string]int, len(memberIDs))
	firstMember := 2 + len(pools)
	for i, id := range memberIDs {
		node := firstMember + i
		memberNodes[id] = node
		graph.link(node, sink, residual[id])
	}
	for i, pool := range pools {
		node := 2 + i
		graph.link(source, node, pool.load)
		for _, id := range pool.members {
			// Keep eligibility edges traversable even when the pool's entire
			// load goes to one member.
			graph.link(node, memberNodes[id], 2*pool.load)
		}
	}

	reachable := graph.maxFlowCut(source, sink)
	// Separate disconnected deficits within the cut so a filtered view can
	// show its own warning without picking up unrelated overloads.
	for start := 2; start < firstMember; start++ {
		if !reachable[start] {
			continue
		}
		group := []int{start}
		reachable[start] = false
		for i := 0; i < len(group); i++ {
			for _, edge := range graph[group[i]] {
				if edge.to >= 2 && reachable[edge.to] {
					group = append(group, edge.to)
					reachable[edge.to] = false
				}
			}
		}
		slices.Sort(group)
		detail := SharedCapacityOverload{Unit: unit}
		visible := false
		for _, node := range group {
			if node < firstMember {
				pool := pools[node-2]
				detail.PoolIDs = append(detail.PoolIDs, pool.id)
				detail.Demand += pool.load
				visible = visible || c.showsRow(c.rows[pool.id+"\x00"+unit])
			} else {
				id := memberIDs[node-firstMember]
				detail.MemberIDs = append(detail.MemberIDs, id)
				detail.UsableCapacity += residual[id]
				visible = visible || c.showsRow(&ResourceWorkload{ResourceID: id, ResourceClass: c.resources[id].Class, Unit: unit})
			}
		}
		detail.Demand = roundDashboardFloat(detail.Demand)
		detail.UsableCapacity = roundDashboardFloat(detail.UsableCapacity)
		detail.Shortfall = roundDashboardFloat(detail.Demand - detail.UsableCapacity)
		if visible && detail.Shortfall > 0 {
			c.result.Warnings = append(c.result.Warnings, Diagnostic{
				Code: "shared_capacity_overload", Severity: "warning",
				Target: Target{Kind: "resource_pool", ID: detail.PoolIDs[0]},
				Message: fmt.Sprintf("pools %s require %g %s but their members have %g available after direct load (%g short)",
					strings.Join(detail.PoolIDs, ", "), detail.Demand, unit, detail.UsableCapacity, detail.Shortfall),
				SharedCapacity: &detail,
			})
		}
	}
}

type capacityFlow [][]capacityEdge

type capacityEdge struct {
	to, reverse int
	remaining   float64
}

func (g capacityFlow) link(from, to int, capacity float64) {
	forward := capacityEdge{to: to, reverse: len(g[to]), remaining: capacity}
	backward := capacityEdge{to: from, reverse: len(g[from])}
	g[from] = append(g[from], forward)
	g[to] = append(g[to], backward)
}

// maxFlowCut uses shortest augmenting paths. When none remains, the nodes
// reachable from the source identify the deficient set of pools and members.
func (g capacityFlow) maxFlowCut(source, sink int) []bool {
	const epsilon = 1e-9 // Well below the workload row resolution of 0.0001.
	seen := make([]bool, len(g))
	from := make([]int, len(g))
	via := make([]int, len(g))
	queue := make([]int, 0, len(g))
	for {
		clear(seen)
		seen[source] = true
		queue = append(queue[:0], source)
		for i := 0; i < len(queue) && !seen[sink]; i++ {
			node := queue[i]
			for j, edge := range g[node] {
				if !seen[edge.to] && edge.remaining > epsilon {
					seen[edge.to] = true
					from[edge.to], via[edge.to] = node, j
					queue = append(queue, edge.to)
				}
			}
		}
		if !seen[sink] {
			return seen
		}
		amount := g[from[sink]][via[sink]].remaining
		for node := sink; node != source; node = from[node] {
			amount = min(amount, g[from[node]][via[node]].remaining)
		}
		for node := sink; node != source; node = from[node] {
			edge := &g[from[node]][via[node]]
			edge.remaining -= amount
			g[node][edge.reverse].remaining += amount
		}
	}
}
