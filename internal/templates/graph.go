package templates

import "fmt"

type Graph struct {
	edges map[string]map[string]struct{}
}

func NewGraph() *Graph {
	return &Graph{edges: make(map[string]map[string]struct{})}
}

func (g *Graph) AddNode(name string) {
	if _, ok := g.edges[name]; !ok {
		g.edges[name] = make(map[string]struct{})
	}
}

func (g *Graph) AddEdge(from, to string) {
	g.AddNode(from)
	g.AddNode(to)
	g.edges[from][to] = struct{}{}
}

func (g *Graph) DetectCycles() error {
	visited := make(map[string]bool)
	onStack := make(map[string]bool)

	var visit func(string) error
	visit = func(node string) error {
		if onStack[node] {
			return fmt.Errorf("cycle detected at %q", node)
		}
		if visited[node] {
			return nil
		}
		visited[node] = true
		onStack[node] = true
		for next := range g.edges[node] {
			if err := visit(next); err != nil {
				return err
			}
		}
		onStack[node] = false
		return nil
	}

	for node := range g.edges {
		if err := visit(node); err != nil {
			return err
		}
	}
	return nil
}
