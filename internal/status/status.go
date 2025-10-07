package status

import (
	"fmt"
	"io"
	"time"
)

type Report struct {
	LastAppliedAt time.Time
	Notes         []string
}

func PrintReport(w io.Writer, r *Report) {
	_, _ = fmt.Fprintf(w, "Applied at: %s", r.LastAppliedAt.Format(time.RFC3339))
	for _, n := range r.Notes {
		_, _ = fmt.Fprintf(w, "- %s", n)
	}
}
