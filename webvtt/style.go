package webvtt

import (
	"fmt"
	"strconv"
	"strings"
)

func parseStyle(doc *Document, lines []string) error {
	css := strings.Join(lines, "\n")
	css = stripCSSComments(css)
	for len(css) > 0 {
		open := strings.IndexByte(css, '{')
		if open < 0 {
			break
		}
		close := strings.IndexByte(css[open+1:], '}')
		if close < 0 {
			return fmt.Errorf("unterminated STYLE rule")
		}
		close += open + 1
		selectors := splitSelectors(css[:open])
		declarations := parseDeclarations(css[open+1 : close])
		for _, selector := range selectors {
			selector = normalizeSelector(selector)
			if selector == "" {
				continue
			}
			rule := StyleRule{
				Selector: selector,
				Color:    declarations.color,
				Margin:   declarations.margin,
			}
			if rule.Color != "" || rule.Margin != (Margin{}) {
				doc.Styles = append(doc.Styles, rule)
			}
		}
		css = css[close+1:]
	}
	return nil
}

type styleDeclarations struct {
	color  string
	margin Margin
}

func parseDeclarations(body string) styleDeclarations {
	result := styleDeclarations{}
	for _, declaration := range strings.Split(body, ";") {
		property, value, ok := strings.Cut(declaration, ":")
		if !ok {
			continue
		}
		property = strings.ToLower(strings.TrimSpace(property))
		value = strings.TrimSpace(value)
		switch property {
		case "color":
			if value != "" {
				result.color = value
			}
		case "margin":
			values := parseMargins(value)
			if len(values) > 0 {
				result.margin = shorthandMargin(values)
			}
		case "margin-top":
			result.margin.Top = parseLength(value)
		case "margin-right":
			result.margin.Right = parseLength(value)
		case "margin-bottom":
			result.margin.Bottom = parseLength(value)
		case "margin-left":
			result.margin.Left = parseLength(value)
		}
	}
	return result
}

func parseMargins(value string) []float64 {
	fields := strings.Fields(value)
	values := make([]float64, 0, len(fields))
	for _, field := range fields {
		parsed, ok := lengthValue(field)
		if !ok {
			return nil
		}
		values = append(values, parsed)
	}
	if len(values) > 4 {
		return nil
	}
	return values
}

func shorthandMargin(values []float64) Margin {
	switch len(values) {
	case 1:
		v := values[0]
		return Margin{Top: v, Right: v, Bottom: v, Left: v}
	case 2:
		return Margin{Top: values[0], Right: values[1], Bottom: values[0], Left: values[1]}
	case 3:
		return Margin{Top: values[0], Right: values[1], Bottom: values[2], Left: values[1]}
	case 4:
		return Margin{Top: values[0], Right: values[1], Bottom: values[2], Left: values[3]}
	default:
		return Margin{}
	}
}

func parseLength(value string) float64 {
	parsed, _ := lengthValue(value)
	return parsed
}

func lengthValue(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "0" {
		return 0, true
	}
	if !strings.HasSuffix(value, "px") {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "px")), 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func stripCSSComments(css string) string {
	for {
		start := strings.Index(css, "/*")
		if start < 0 {
			return css
		}
		end := strings.Index(css[start+2:], "*/")
		if end < 0 {
			return css[:start]
		}
		css = css[:start] + css[start+2+end+2:]
	}
}

func splitSelectors(value string) []string {
	var selectors []string
	for _, selector := range strings.Split(value, ",") {
		if selector = normalizeSelector(selector); selector != "" {
			selectors = append(selectors, selector)
		}
	}
	return selectors
}

func normalizeSelector(value string) string {
	value = strings.Join(strings.Fields(value), "")
	if strings.HasPrefix(value, "::cue.") {
		return "." + strings.TrimPrefix(value, "::cue.")
	}
	if strings.HasPrefix(value, "::cue(") && strings.HasSuffix(value, ")") {
		inner := strings.TrimSuffix(strings.TrimPrefix(value, "::cue("), ")")
		if strings.HasPrefix(inner, ".") {
			return inner
		}
	}
	switch value {
	case "::cue", ":cue":
		return "::cue"
	}
	if strings.HasPrefix(value, ".") {
		return value
	}
	return ""
}

// ResolveStyle applies ::cue first and then the cue's classes in source order.
func ResolveStyle(rules []StyleRule, classes []string) (ResolvedStyle, []string) {
	style := ResolvedStyle{Color: "#f5f7ff"}
	known := make(map[string]bool)
	for _, rule := range rules {
		if strings.HasPrefix(rule.Selector, ".") {
			known[strings.TrimPrefix(rule.Selector, ".")] = true
		}
	}
	var missing []string
	for _, class := range classes {
		if !known[class] {
			missing = append(missing, class)
		}
	}
	for _, rule := range rules {
		if rule.Selector == "::cue" {
			applyStyleRule(&style, rule)
		}
	}
	for _, class := range classes {
		selector := "." + class
		for _, rule := range rules {
			if rule.Selector == selector {
				applyStyleRule(&style, rule)
			}
		}
	}
	return style, missing
}

func applyStyleRule(style *ResolvedStyle, rule StyleRule) {
	if rule.Color != "" {
		style.Color = rule.Color
	}
	if rule.Margin != (Margin{}) {
		style.Margin = rule.Margin
	}
}
