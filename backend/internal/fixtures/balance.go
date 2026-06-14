package fixtures

// orientBalanced returns, for every unordered pair {i,j} (i<j) of participants
// 0..n-1, whether i is the home side. The orientation guarantees each
// participant's home and away counts differ by at most 1.
//
// It is an Eulerian orientation of the complete graph K_n: traversing an Euler
// circuit and orienting each edge in the direction of travel makes in-degree =
// out-degree at every vertex. When n is even (each vertex has odd degree n-1) a
// dummy vertex is added so a circuit exists; dropping its edges shifts the
// incident real vertices by at most 1 — hence the ±1 bound. For odd n the bound
// is exact (0). Deterministic: the graph and traversal are built in fixed order.
func orientBalanced(n int) map[[2]int]bool {
	type edge struct{ to, id int }
	v := n
	addDummy := n%2 == 0
	if addDummy {
		v = n + 1
	}
	adj := make([][]edge, v)
	var used []bool
	addEdge := func(a, b int) {
		id := len(used)
		used = append(used, false)
		adj[a] = append(adj[a], edge{to: b, id: id})
		adj[b] = append(adj[b], edge{to: a, id: id})
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			addEdge(i, j)
		}
	}
	if addDummy {
		for i := 0; i < n; i++ {
			addEdge(i, n) // dummy vertex n evens out every real vertex's degree
		}
	}

	// Hierholzer: produce an Euler circuit as a vertex sequence.
	iter := make([]int, v)
	stack := []int{0}
	var path []int
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		for iter[x] < len(adj[x]) && used[adj[x][iter[x]].id] {
			iter[x]++
		}
		if iter[x] == len(adj[x]) {
			path = append(path, x)
			stack = stack[:len(stack)-1]
		} else {
			e := adj[x][iter[x]]
			used[e.id] = true
			iter[x]++
			stack = append(stack, e.to)
		}
	}

	// Consecutive vertices in the circuit are an edge, oriented in travel order.
	orient := make(map[[2]int]bool, n*(n-1)/2)
	for k := 0; k+1 < len(path); k++ {
		u, w := path[k], path[k+1]
		if u >= n || w >= n {
			continue // skip dummy edges
		}
		lo, hi := u, w
		if lo > hi {
			lo, hi = hi, lo
		}
		if _, seen := orient[[2]int{lo, hi}]; !seen {
			orient[[2]int{lo, hi}] = (u == lo) // home is the travel source
		}
	}
	return orient
}

// homeAway applies a balanced orientation to a circle-method pair (indices a,b).
func homeAway(orient map[[2]int]bool, a, b int) (home, away int) {
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	if orient[[2]int{lo, hi}] {
		return lo, hi
	}
	return hi, lo
}
