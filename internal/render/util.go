package render

import (
	"encoding/json"
	"strings"
)

// jsonOrEmpty marshals v to JSON. On error, returns the empty marshalled
// slice/object so the embedded HTML template can still render.
func jsonOrEmpty(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]", err
	}
	return string(b), nil
}

// substituteTemplate replaces the {{NODES}} and {{EDGES}} placeholders
// in the HTML template with the JSON-marshalled projections.
func substituteTemplate(tmpl string, nodes []HTMLNode, edges []HTMLEdge) (string, error) {
	nodesJSON, err := jsonOrEmpty(nodes)
	if err != nil {
		return "", err
	}
	edgesJSON, err := jsonOrEmpty(edges)
	if err != nil {
		return "", err
	}
	tmpl = strings.Replace(tmpl, "{{NODES}}", nodesJSON, 1)
	tmpl = strings.Replace(tmpl, "{{EDGES}}", edgesJSON, 1)
	return tmpl, nil
}
