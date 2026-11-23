package wav

import (
	"fmt"
	"strings"
)

// ValidationError reports every validation finding found while parsing.
// Issues is ordered by FirstSampleOffset and then by discovery order.
type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return ""
	}
	var b strings.Builder
	for i, issue := range e.Issues {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s at sample offset %d: %s", issue.Code, issue.FirstSampleOffset, issue.Message)
	}
	return b.String()
}

// First returns the first error-severity issue, if any.
func (e *ValidationError) First() *Issue {
	if e == nil {
		return nil
	}
	for i := range e.Issues {
		if e.Issues[i].Severity == "error" {
			return &e.Issues[i]
		}
	}
	return nil
}
