package config

type LoadTrace struct {
	FieldPath     []string
	Layers        []ExplainLayer
	ImportLayers  []ExplainLayer
	OverlayLayers []ExplainLayer
	MergedDoc     map[string]any
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

func (t *LoadTrace) recordOverlayLayers(layers []ExplainLayer) {
	if t == nil || len(layers) == 0 {
		return
	}
	t.OverlayLayers = append(t.OverlayLayers, layers...)
}

func (t *LoadTrace) recordOverlay(label string, value any) {
	if t == nil || label == "" {
		return
	}
	t.OverlayLayers = append(t.OverlayLayers, ExplainLayer{
		Label: label,
		Value: value,
	})
}
