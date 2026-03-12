package config

type LoadTrace struct {
	FieldPath    []string
	Layers       []ExplainLayer
	ImportLayers []ExplainLayer
	MergedDoc    map[string]any
}

func (t *LoadTrace) record(label string, document map[string]any) {
	if t == nil || len(t.FieldPath) == 0 || document == nil {
		return
	}
	if value, ok := lookupPathValue(document, t.FieldPath); ok {
		t.Layers = append(t.Layers, ExplainLayer{
			Label: label,
			Value: value,
		})
	}
}

func (t *LoadTrace) recordImport(label string, document map[string]any, fieldPath []string) {
	if t == nil || len(fieldPath) == 0 || document == nil {
		return
	}
	if value, ok := lookupPathValue(document, fieldPath); ok {
		t.ImportLayers = append(t.ImportLayers, ExplainLayer{
			Label: label,
			Value: value,
		})
	}
}
