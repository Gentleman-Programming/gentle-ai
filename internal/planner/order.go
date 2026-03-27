package planner

import (
	"errors"
	"fmt"
	"slices"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

var ErrDependencyCycle = errors.New("dependency cycle detected")

func TopologicalSort(dependencies map[model.ComponentID][]model.ComponentID) ([]model.ComponentID, error) {
	nodes := make(map[model.ComponentID]struct{}, len(dependencies))
	inDegree := make(map[model.ComponentID]int, len(dependencies))
	children := make(map[model.ComponentID][]model.ComponentID, len(dependencies))

	for component, deps := range dependencies {
		nodes[component] = struct{}{}
		if _, ok := inDegree[component]; !ok {
			inDegree[component] = 0
		}

		for _, dep := range deps {
			nodes[dep] = struct{}{}
			inDegree[component]++
			children[dep] = append(children[dep], component)
			if _, ok := inDegree[dep]; !ok {
				inDegree[dep] = 0
			}
		}
	}

	queue := make([]model.ComponentID, 0, len(nodes))
	for node := range nodes {
		if inDegree[node] == 0 {
			queue = append(queue, node)
		}
	}
	slices.Sort(queue)

	ordered := make([]model.ComponentID, 0, len(nodes))
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		ordered = append(ordered, node)

		slices.Sort(children[node])
		for _, child := range children[node] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
				slices.Sort(queue)
			}
		}
	}

	if len(ordered) != len(nodes) {
		return nil, fmt.Errorf("%w: unresolved graph", ErrDependencyCycle)
	}

	return ordered, nil
}

// applySoftOrdering reorders an already topologically-sorted slice so that the
// first component in each pair appears before the second WHEN BOTH are already
// present. It never inserts missing components, so it cannot create new hard
// dependencies.
//
// SAFETY CONTRACT: the `first` element in each pair MUST have no hard
// dependencies in the plan — otherwise moving it earlier could place it before
// one of its own transitive requirements, silently breaking topological order.
// Today all soft pairs use ComponentPersona (which has nil deps) as `first`.
// If you add a pair where `first` has deps, you must add a topo-validation
// step after the reorder.
func applySoftOrdering(ordered []model.ComponentID, pairs [][2]model.ComponentID) []model.ComponentID {
	result := make([]model.ComponentID, len(ordered))
	copy(result, ordered)

	indexOf := func(items []model.ComponentID, target model.ComponentID) int {
		for i, item := range items {
			if item == target {
				return i
			}
		}
		return -1
	}

	for _, pair := range pairs {
		first, second := pair[0], pair[1]
		i := indexOf(result, first)
		j := indexOf(result, second)
		if i < 0 || j < 0 || i < j {
			continue
		}

		// Move first to just before second, preserving relative order of others.
		item := result[i]
		copy(result[j+1:i+1], result[j:i])
		result[j] = item
	}

	return result
}
