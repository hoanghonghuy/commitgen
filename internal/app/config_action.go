package app

import "strings"

// ResolveConfigAction extracts show|path from CLI args for both
// `commitgen config show` and `commitgen -cmd=config show`.
func ResolveConfigAction(args []string) string {
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "show", "path":
			return strings.ToLower(strings.TrimSpace(a))
		}
	}
	return ""
}
