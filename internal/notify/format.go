package notify

import "strings"

func BulletList(items []string) string {
	if len(items) == 0 {
		return "  (none)\n"
	}
	var b strings.Builder
	for _, item := range items {
		b.WriteString("  • ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	return b.String()
}

func Checkmark(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}
