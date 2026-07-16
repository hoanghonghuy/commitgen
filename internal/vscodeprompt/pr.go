package vscodeprompt

import (
	"strings"
)

// BuildPRMessages builds system+user messages for generating a PR title and description
// from the merge-base range (branch commits + file diffs).
func BuildPRMessages(d Data) []VSCodeMessage {
	tmpl := d.SystemPromptTemplate
	if tmpl == "" {
		tmpl = defaultPRPromptTemplate()
	}
	systemText := renderTemplate(tmpl, d)
	userText := buildPRUserText(d)
	return []VSCodeMessage{
		{Role: RoleSystem, Content: []VSCodeContentPart{{Type: 1, Text: systemText}}},
		{Role: RoleUser, Content: []VSCodeContentPart{{Type: 1, Text: userText}}},
	}
}

func defaultPRPromptTemplate() string {
	return "" +
		"You are an AI assistant helping a software developer write a GitHub/GitLab pull request.\n" +
		"You analyze commits and code changes since the merge-base with the target branch.\n\n" +
		"# Goals:\n" +
		"- Write a clear, concise PR **title** (one line, imperative mood).\n" +
		"- Write a useful PR **description** with Summary and Test plan sections.\n" +
		"- Focus on *why* and user-facing impact, not a line-by-line changelog.\n" +
		"- Prefer Conventional Commits style for the title when it fits (feat/fix/docs/…).\n\n" +
		"# Output format (STRICT):\n" +
		"Return ONLY one markdown code block:\n" +
		"```markdown\n" +
		"# <PR title>\n" +
		"\n" +
		"## Summary\n" +
		"- …\n" +
		"\n" +
		"## Test plan\n" +
		"- [ ] …\n" +
		"```\n"
}

func buildPRUserText(d Data) string {
	var b strings.Builder

	b.WriteString("<pull-request-context>\n")
	b.WriteString("Repository: ")
	b.WriteString(d.RepositoryName)
	b.WriteString("\n")
	b.WriteString("Head branch: ")
	b.WriteString(d.BranchName)
	b.WriteString("\n")
	b.WriteString("Base branch: ")
	b.WriteString(d.BaseBranch)
	b.WriteString("\n</pull-request-context>\n")

	if len(d.RecentRepoCommits) > 0 {
		b.WriteString("<commits-since-base>\n")
		for _, c := range d.RecentRepoCommits {
			b.WriteString("- ")
			b.WriteString(c)
			b.WriteString("\n")
		}
		b.WriteString("</commits-since-base>\n")
	}

	b.WriteString("<code-changes>\n")
	for _, ch := range d.Changes {
		b.WriteString("## File: ")
		b.WriteString(ch.Path)
		b.WriteString("\n")
		if strings.TrimSpace(ch.OriginalCode) != "" {
			b.WriteString(ch.OriginalCode)
			b.WriteString("\n")
		}
		b.WriteString("### Diff\n")
		b.WriteString(ch.Diff)
		b.WriteString("\n")
	}
	b.WriteString("</code-changes>\n")

	b.WriteString("<custom-instructions>\n")
	if strings.TrimSpace(d.CustomInstructions) != "" {
		b.WriteString(strings.TrimRight(d.CustomInstructions, "\n"))
		b.WriteString("\n")
	}
	b.WriteString("</custom-instructions>\n")

	b.WriteString("<language>\n")
	switch strings.ToLower(d.ReviewLanguage) {
	case "vi":
		b.WriteString("# Write the PR title and description in Vietnamese (Tiếng Việt).\n")
		b.WriteString("# Keep code identifiers in their original form.\n")
	case "ja":
		b.WriteString("# Write the PR title and description in Japanese (日本語).\n")
		b.WriteString("# Keep code identifiers in their original form.\n")
	case "zh":
		b.WriteString("# Write the PR title and description in Chinese (中文).\n")
		b.WriteString("# Keep code identifiers in their original form.\n")
	default:
		b.WriteString("# Write the PR title and description in English.\n")
	}
	b.WriteString("</language>\n")

	b.WriteString("<reminder>\n")
	b.WriteString("Return ONLY a single ```markdown code block with a # title, ## Summary, and ## Test plan.\n")
	b.WriteString("</reminder>\n")

	return b.String()
}

// ParsePROutput extracts a PR title and body from model output.
// Prefers a fenced markdown block whose first heading is the title.
func ParsePROutput(raw string) (title, body string, ok bool) {
	content, _ := ExtractOneTextCodeBlock(raw)
	content = strings.TrimSpace(content)
	if content == "" {
		return "", "", false
	}

	lines := strings.Split(content, "\n")
	var titleLine string
	var bodyStart int
	for i, ln := range lines {
		trim := strings.TrimSpace(ln)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "# ") {
			titleLine = strings.TrimSpace(strings.TrimPrefix(trim, "# "))
			bodyStart = i + 1
			break
		}
		// First non-empty line without # is treated as title.
		titleLine = trim
		bodyStart = i + 1
		break
	}
	if titleLine == "" {
		return "", "", false
	}

	body = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	return titleLine, body, true
}
