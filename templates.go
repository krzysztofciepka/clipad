package main

import (
	"regexp"
	"time"
)

// templateVarRe matches {{name}} and {{name:layout}} for the supported
// variable names only. Unknown placeholders never match and pass through
// untouched. The layout cannot contain braces (Go reference layouts never do).
var templateVarRe = regexp.MustCompile(`\{\{(date|time|yesterday|vault)(?::([^{}]*))?\}\}`)

// renderTemplate substitutes template variables in content. now is injected so
// rendering is deterministic and unit-testable.
func renderTemplate(content string, now time.Time, vault string) string {
	return templateVarRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := templateVarRe.FindStringSubmatch(match)
		name, layout := sub[1], sub[2]
		switch name {
		case "date":
			if layout != "" {
				return now.Format(layout)
			}
			return now.Format("2006-01-02")
		case "time":
			return now.Format("15:04")
		case "yesterday":
			return now.AddDate(0, 0, -1).Format("2006-01-02")
		case "vault":
			return vault
		}
		return match
	})
}
