package analyze

import (
	"database/sql"
	"sort"

	"github.com/sourcegraph/scip-cli-go/internal/symbols"
)

var FileEdgesSQL = `
    SELECT DISTINCT d1.relative_path AS from_file, d2.relative_path AS to_file
    FROM mentions m
    JOIN chunks c ON m.chunk_id = c.id
    JOIN documents d1 ON c.document_id = d1.id
    JOIN defn_enclosing_ranges der ON m.symbol_id = der.symbol_id
    JOIN documents d2 ON der.document_id = d2.id
    JOIN global_symbols gs ON gs.id = der.symbol_id
    WHERE d1.id != d2.id AND m.role != 1
      AND ` + symbols.CycleRuntimeEdgeSQL("")

type FileEdge struct {
	From string
	To   string
}

func FetchFileEdges(db *sql.DB) ([]FileEdge, error) {
	rows, err := FetchAll(db, FileEdgesSQL)
	if err != nil {
		return nil, err
	}
	var edges []FileEdge
	for _, row := range rows {
		edges = append(edges, FileEdge{
			From: row[0].(string),
			To:   row[1].(string),
		})
	}
	return edges, nil
}

// tarjanSCCs implements iterative Tarjan's algorithm to find strongly connected components.
func tarjanSCCs(graph map[string][]string, nodes map[string]bool) [][]string {
	index := 0
	stack := []string{}
	onStack := map[string]bool{}
	indices := map[string]int{}
	lowlink := map[string]int{}
	var sccs [][]string

	// Iterative version using explicit call stack
	type frame struct {
		vertex string
		neigh  []string
		idx    int
	}

	for node := range nodes {
		if _, visited := indices[node]; visited {
			continue
		}

		callStack := []frame{{vertex: node, neigh: graph[node], idx: 0}}
		indices[node] = index
		lowlink[node] = index
		index++
		stack = append(stack, node)
		onStack[node] = true

		for len(callStack) > 0 {
			top := &callStack[len(callStack)-1]

			if top.idx < len(top.neigh) {
				neighbor := top.neigh[top.idx]
				top.idx++

				if _, visited := indices[neighbor]; !visited {
					indices[neighbor] = index
					lowlink[neighbor] = index
					index++
					stack = append(stack, neighbor)
					onStack[neighbor] = true
					callStack = append(callStack, frame{vertex: neighbor, neigh: graph[neighbor], idx: 0})
				} else if onStack[neighbor] {
					if indices[neighbor] < lowlink[top.vertex] {
						lowlink[top.vertex] = indices[neighbor]
					}
				}
			} else {
				// Done with this vertex
				if lowlink[top.vertex] == indices[top.vertex] {
					var component []string
					for {
						w := stack[len(stack)-1]
						stack = stack[:len(stack)-1]
						delete(onStack, w)
						component = append(component, w)
						if w == top.vertex {
							break
						}
					}
					sccs = append(sccs, component)
				}

				callStack = callStack[:len(callStack)-1]
				if len(callStack) > 0 {
					parent := callStack[len(callStack)-1].vertex
					if lowlink[top.vertex] < lowlink[parent] {
						lowlink[parent] = lowlink[top.vertex]
					}
				}
			}
		}
	}

	return sccs
}

func cyclesInSCC(graph map[string][]string, sccNodes []string, maxDepth, limit int) []string {
	if len(sccNodes) <= 2 {
		return nil
	}

	sccSet := map[string]bool{}
	for _, n := range sccNodes {
		sccSet[n] = true
	}

	subgraph := map[string][]string{}
	for src := range sccSet {
		for _, dst := range graph[src] {
			if sccSet[dst] {
				subgraph[src] = append(subgraph[src], dst)
			}
		}
	}

	found := map[[8]string]string{}

	record := func(path []string) {
		minRot := path
		for i := 1; i < len(path); i++ {
			rot := append(path[i:], path[:i]...)
			if comparePaths(rot, minRot) < 0 {
				minRot = rot
			}
		}
		var key [8]string
		copy(key[:], minRot)
		if _, ok := found[key]; !ok {
			found[key] = joinCycle(path)
		}
	}

	sorted := make([]string, len(sccNodes))
	copy(sorted, sccNodes)
	sort.Strings(sorted)

	for _, origin := range sorted {
		type state struct {
			node string
			path []string
		}
		stack := []state{{origin, []string{origin}}}

		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			if len(cur.path) > maxDepth {
				continue
			}

			for _, nxt := range subgraph[cur.node] {
				if nxt == origin && len(cur.path) >= 2 {
					record(cur.path)
					if len(found) >= limit {
						return sortedCycleValues(found, limit)
					}
				} else if !contains(cur.path, nxt) {
					newPath := make([]string, len(cur.path)+1)
					copy(newPath, cur.path)
					newPath[len(cur.path)] = nxt
					stack = append(stack, state{nxt, newPath})
				}
			}
		}
	}

	return sortedCycleValues(found, limit)
}

func comparePaths(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

func joinCycle(path []string) string {
	result := ""
	for i, p := range path {
		if i > 0 {
			result += " -> "
		}
		result += p
	}
	result += " -> " + path[0]
	return result
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func sortedCycleValues(found map[[8]string]string, limit int) []string {
	var result []string
	for _, v := range found {
		result = append(result, v)
	}
	sort.Strings(result)
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func FindLongerCycles(edges []FileEdge, maxDepth, limit int) ([]string, error) {
	if limit <= 0 || len(edges) == 0 {
		return nil, nil
	}

	graph := map[string][]string{}
	nodes := map[string]bool{}
	for _, e := range edges {
		graph[e.From] = append(graph[e.From], e.To)
		nodes[e.From] = true
		nodes[e.To] = true
	}

	found := map[[8]string]string{}

	for _, component := range tarjanSCCs(graph, nodes) {
		if len(component) <= 2 {
			continue
		}

		for _, line := range cyclesInSCC(graph, component, maxDepth, limit) {
			path := splitCycleLine(line)
			body := path[:len(path)-1]
			minRot := body
			for i := 1; i < len(body); i++ {
				rot := append(body[i:], body[:i]...)
				if comparePaths(rot, minRot) < 0 {
					minRot = rot
				}
			}
			var key [8]string
			copy(key[:], minRot)
			if _, ok := found[key]; !ok {
				found[key] = line
			}
			if len(found) >= limit {
				return sortedCycleValues(found, limit), nil
			}
		}
	}

	return sortedCycleValues(found, limit), nil
}

func splitCycleLine(line string) []string {
	parts := []string{}
	current := ""
	for i := 0; i < len(line); i++ {
		if i+3 < len(line) && line[i:i+4] == " -> " {
			parts = append(parts, current)
			current = ""
			i += 3
		} else {
			current += string(line[i])
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
