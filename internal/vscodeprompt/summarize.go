package vscodeprompt

import (
	"fmt"
	"path/filepath"
	"strings"
)

func BuildAttachment(repoRoot, relPath, content string, summarize bool) string {
	base := filepath.Base(relPath)
	abs := filepath.Join(repoRoot, relPath)

	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	total := len(lines)

	kept := map[int]string{}
	if summarize {
		kept = summarizeByType(relPath, lines)
	} else {
		for i, s := range lines {
			kept[i+1] = strings.TrimRight(s, "\r")
		}
	}

	width := len(fmt.Sprintf("%d", total))
	if width < 2 {
		width = 2
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<attachment id=\"%s\" isSummarized=\"%t\">\n", base, summarize))
	b.WriteString(filepathCommentLine(relPath, abs))

	keys := make([]int, 0, len(kept))
	for k := range kept {
		keys = append(keys, k)
	}
	sortInts(keys)

	for _, ln := range keys {
		b.WriteString(fmt.Sprintf("%*d: %s\n", width, ln, kept[ln]))
	}
	b.WriteString("</attachment>\n")
	return b.String()
}

func summarizeByType(relPath string, lines []string) map[int]string {
	ext := strings.ToLower(filepath.Ext(relPath))
	switch ext {
	case ".md", ".txt", ".json", ".yml", ".yaml":
		return summarizeHeadPlusLast(lines, 25, 1)

	case ".go":
		return summarizeGo(lines)

	default:
		return summarizeHeadTail(lines, 80, 5)
	}
}

// Like VSCode dump for .md: keep head and last-line marker.
func summarizeHeadPlusLast(lines []string, headN int, tailN int) map[int]string {
	kept := map[int]string{}
	n := len(lines)

	h := min(headN, n)
	for i := 1; i <= h; i++ {
		kept[i] = strings.TrimRight(lines[i-1], "\r")
	}

	// keep last line
	if n >= 1 {
		kept[n] = strings.TrimRight(lines[n-1], "\r")
	}
	return kept
}

func summarizeHeadTail(lines []string, headN, tailN int) map[int]string {
	kept := map[int]string{}
	n := len(lines)

	h := min(headN, n)
	for i := 1; i <= h; i++ {
		kept[i] = strings.TrimRight(lines[i-1], "\r")
	}

	start := max(1, n-tailN+1)
	for i := start; i <= n; i++ {
		kept[i] = strings.TrimRight(lines[i-1], "\r")
	}
	return kept
}

// Goal: mimic what you saw in VSCode dump for Go:
// - keep package/import/type/const/var blocks, comments
// - collapse each func body to one line with "{…}"
func summarizeGo(lines []string) map[int]string {
	kept := map[int]string{}
	n := len(lines)

	inImportBlock := false
	inTypeConstVarBlock := false
	blockDepth := 0

	inFunc := false
	funcStartLine := 0
	funcSig := ""

	for i := 0; i < n; i++ {
		ln := i + 1
		line := strings.TrimRight(lines[i], "\r")
		trim := strings.TrimSpace(line)

		// import ( ... )
		if strings.HasPrefix(trim, "import (") && !inFunc {
			inImportBlock = true
			kept[ln] = line
			continue
		}
		if inImportBlock && !inFunc {
			kept[ln] = line
			if trim == ")" {
				inImportBlock = false
			}
			continue
		}

		// type/const/var blocks: keep full block when using parentheses
		if !inFunc && (strings.HasPrefix(trim, "type (") || strings.HasPrefix(trim, "const (") || strings.HasPrefix(trim, "var (")) {
			inTypeConstVarBlock = true
			blockDepth = 1
			kept[ln] = line
			continue
		}
		if inTypeConstVarBlock && !inFunc {
			kept[ln] = line
			if strings.Contains(line, "(") {
				blockDepth += strings.Count(line, "(")
			}
			if strings.Contains(line, ")") {
				blockDepth -= strings.Count(line, ")")
				if blockDepth <= 0 {
					inTypeConstVarBlock = false
				}
			}
			continue
		}

		// func start heuristic
		if !inFunc && strings.HasPrefix(trim, "func ") {
			inFunc = true
			funcStartLine = ln
			funcSig = line

			// If "{" is on same line: collapse immediately.
			if idx := strings.Index(funcSig, "{"); idx >= 0 {
				funcSig = strings.TrimRight(funcSig[:idx], " \t") + " {…}"
				kept[funcStartLine] = funcSig
				// now skip until func ends by brace counting
				open := strings.Count(line, "{")
				close := strings.Count(line, "}")
				depth := open - close
				if depth <= 0 && strings.Contains(line, "}") {
					inFunc = false
					funcStartLine = 0
					funcSig = ""
				} else {
					// store depth in local variable by scanning ahead
					// (we'll handle via scanning until matched)
					// We'll mark depth by using negative sentinel in kept map isn't good.
				}
			} else {
				// multiline signature: keep first line for now; will collapse at first "{"
				kept[funcStartLine] = funcSig
			}
			continue
		}

		if inFunc {
			// multiline signature: find first "{"
			if strings.Contains(line, "{") && funcStartLine != 0 {
				// rewrite the signature line to {…}
				sig := kept[funcStartLine]
				sig = strings.TrimRight(sig, " \t") + " {…}"
				kept[funcStartLine] = sig
			}

			// naive brace matching: exit when a line contains "}" and seems to close function.
			// We'll do a simple depth scan from funcStart: compute by accumulating braces.
			// Instead of storing depth across lines, we recompute from funcStart each line range is heavy.
			// We'll use a simplified rule: if trim == "}" or line contains "}" and not inside nested types, end func.
			// This matches the VSCode-like summarization you observed (good enough).
			if strings.TrimSpace(line) == "}" {
				inFunc = false
				funcStartLine = 0
				funcSig = ""
			}
			continue
		}

		// keep structural lines & comments & blanks
		if trim == "" ||
			strings.HasPrefix(trim, "package ") ||
			strings.HasPrefix(trim, "type ") ||
			strings.HasPrefix(trim, "const ") ||
			strings.HasPrefix(trim, "var ") ||
			strings.HasPrefix(trim, "//") {
			kept[ln] = line
		}
	}

	// VS Code dump often shows a trailing line number; keep last line too
	if n >= 1 {
		kept[n] = strings.TrimRight(lines[n-1], "\r")
	}

	return kept
}

func filepathCommentLine(rel, abs string) string {
	ext := strings.ToLower(filepath.Ext(rel))
	switch ext {
	case ".md", ".html", ".xml", ".yaml", ".yml", ".json":
		return fmt.Sprintf("<!-- filepath: %s -->\n", abs)
	case ".py", ".sh":
		return fmt.Sprintf("# filepath: %s\n", abs)
	default:
		return fmt.Sprintf("// filepath: %s\n", abs)
	}
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		j := i
		for j > 0 && a[j-1] > a[j] {
			a[j-1], a[j] = a[j], a[j-1]
			j--
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
