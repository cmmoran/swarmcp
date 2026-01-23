package templates

import (
	"strconv"
	"strings"
)

func EscapeTemplate(args ...string) string {
	// defaults
	input := ""
	levels := 1
	openDelim := "{{"
	closeDelim := "}}"

	if len(args) >= 1 {
		input = args[0]
	}

	switch len(args) {
	case 0, 1:
		// defaults only
	case 2:
		if n, err := strconv.Atoi(args[1]); err == nil && n > 0 {
			levels = n
		} else if args[1] != "" {
			openDelim = args[1]
		}

	case 3:
		if n, err := strconv.Atoi(args[1]); err == nil && n > 0 {
			levels = n
			if args[2] != "" {
				openDelim = args[2]
			}
		} else {
			if args[1] != "" {
				openDelim = args[1]
			}
			if args[2] != "" {
				closeDelim = args[2]
			}
		}

	default:
		// len >= 4
		if n, err := strconv.Atoi(args[1]); err == nil && n > 0 {
			levels = n
			if args[2] != "" {
				openDelim = args[2]
			}
			if args[3] != "" {
				closeDelim = args[3]
			}
		} else {
			if args[1] != "" {
				openDelim = args[1]
			}
			if args[2] != "" {
				closeDelim = args[2]
			}
		}
	}

	if input == "" {
		return wrapEscapedTemplate(input, levels, openDelim, closeDelim)
	}

	if hasTemplateExpr(input, openDelim, closeDelim) {
		return escapeTemplateExpressions(input, levels, openDelim, closeDelim)
	}

	expr := "{{ " + input + " }}"
	if levels <= 1 {
		return expr
	}
	return wrapEscapedTemplate(expr, levels-1, openDelim, closeDelim)
}

func EscapeSwarmTemplate(args ...string) string {
	return EscapeTemplate(args...)
}

func wrapEscapedTemplate(input string, levels int, openDelim, closeDelim string) string {
	escaped := input
	for i := 0; i < levels; i++ {
		escaped = wrapEscapedTemplateOnce(escaped, openDelim, closeDelim)
	}
	return escaped
}

func wrapEscapedTemplateOnce(input string, openDelim, closeDelim string) string {
	escaped := strconv.Quote(input)
	return openDelim + " " + escaped + " " + closeDelim
}

func hasTemplateExpr(input, openDelim, closeDelim string) bool {
	open := strings.Index(input, openDelim)
	if open == -1 {
		return false
	}
	close := strings.Index(input[open+len(openDelim):], closeDelim)
	return close != -1
}

func escapeTemplateExpressions(input string, levels int, openDelim, closeDelim string) string {
	if levels <= 1 {
		return input
	}

	var out strings.Builder
	rest := input

	for {
		open := strings.Index(rest, openDelim)
		if open == -1 {
			out.WriteString(rest)
			break
		}
		out.WriteString(rest[:open])

		afterOpen := rest[open+len(openDelim):]
		close := strings.Index(afterOpen, closeDelim)
		if close == -1 {
			out.WriteString(rest[open:])
			break
		}

		expr := rest[open : open+len(openDelim)+close+len(closeDelim)]
		out.WriteString(wrapEscapedTemplate(expr, levels-1, openDelim, closeDelim))
		rest = afterOpen[close+len(closeDelim):]
	}

	return out.String()
}
