package swarm

// Deterministic names + labels for ownership & GC.

const (
	LabelProject     = "swarmcp.project"
	LabelStack       = "swarmcp.stack"
	LabelInstance    = "swarmcp.instance"
	LabelService     = "swarmcp.service"
	LabelGeneration  = "swarmcp.generation"
	LabelFingerprint = "swarmcp.fingerprint"
	LabelOwner       = "swarmcp.owner" // constant: "swarmcp"
)

type NameParts struct {
	Project  string
	Stack    string
	Instance string // empty for shared
	Service  string
}

// ServiceName returns the deterministic Swarm service name.
func ServiceName(p NameParts) string {
	base := "proj." + p.Project + ".stack." + p.Stack
	if p.Instance != "" {
		base += ".inst." + p.Instance
	}
	return base + ".svc." + p.Service
}

// ObjectLabels returns the standard label set for all owned objects.
func ObjectLabels(p NameParts, generation uint64, fingerprint string) map[string]string {
	lbls := map[string]string{
		LabelOwner:      "swarmcp",
		LabelProject:    p.Project,
		LabelStack:      p.Stack,
		LabelService:    p.Service,
		LabelGeneration: uintToStr(generation),
	}
	if p.Instance != "" {
		lbls[LabelInstance] = p.Instance
	}
	if fingerprint != "" {
		lbls[LabelFingerprint] = fingerprint
	}
	return lbls
}

func uintToStr(u uint64) string {
	const digits = "0123456789"
	if u == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for u > 0 {
		i--
		buf[i] = digits[u%10]
		u /= 10
	}
	return string(buf[i:])
}
