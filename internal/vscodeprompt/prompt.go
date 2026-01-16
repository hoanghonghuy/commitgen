package vscodeprompt

import (
	"fmt"
	"regexp"
	"strings"
)

type VSCodeContentPart struct {
	Type int    `json:"type"` // 1=text
	Text string `json:"text"`
}

type VSCodeMessage struct {
	Role    int                 `json:"role"` // 0=system, 1=user
	Content []VSCodeContentPart `json:"content"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`    // system|user
	Content string `json:"content"` // string
}

type Change struct {
	Path         string
	Diff         string
	OriginalCode string // already attachment-wrapped and numbered
}

type Data struct {
	RepositoryName       string
	BranchName           string
	RecentUserCommits    []string
	RecentRepoCommits    []string
	Changes              []Change
	CustomInstructions   string
	SummarizeAttachments bool
}

func BuildVSCodeMessages(d Data) []VSCodeMessage {
	systemText := systemPromptText()
	userText := buildUserText(d)

	return []VSCodeMessage{
		{
			Role: 0,
			Content: []VSCodeContentPart{
				{Type: 1, Text: systemText},
			},
		},
		{
			Role: 1,
			Content: []VSCodeContentPart{
				{Type: 1, Text: userText},
			},
		},
	}
}

// This is copied to match the prompt you dumped from VS Code (including policy lines).
func systemPromptText() string {
	return "" +
		"You are an AI programming assistant, helping a software developer to come with the best git commit message for their code changes.\n" +
		"You excel in interpreting the purpose behind code changes to craft succinct, clear commit messages that adhere to the repository's guidelines.\n\n" +
		"# First, think step-by-step:\n" +
		"1. Analyze the CODE CHANGES thoroughly to understand what's been modified.\n" +
		"2. Use the ORIGINAL CODE to understand the context of the CODE CHANGES. Use the line numbers to map the CODE CHANGES to the ORIGINAL CODE.\n" +
		"3. Identify the purpose of the changes to answer the *why* for the commit messages, also considering the optionally provided RECENT USER COMMITS.\n" +
		"4. Review the provided RECENT REPOSITORY COMMITS to identify established commit message conventions. Focus on the format and style, ignoring commit-specific details like refs, tags, and authors.\n" +
		"5. Generate a thoughtful and succinct commit message for the given CODE CHANGES. It MUST follow the the established writing conventions. 6. Remove any meta information like issue references, tags, or author names from the commit message. The developer will add them.\n" +
		"7. Now only show your message, wrapped with a single markdown ```text codeblock! Do not provide any explanations or details\n" +
		"Follow Microsoft content policies.\n" +
		"Avoid content that violates copyrights.\n" +
		"If you are asked to generate content that is harmful, hateful, racist, sexist, lewd, or violent, only respond with \"Sorry, I can't assist with that.\"\n" +
		"Keep your answers short and impersonal.\n"
}

func buildUserText(d Data) string {
	var b strings.Builder

	b.WriteString("<repository-context>\n")
	b.WriteString("# REPOSITORY DETAILS:\n")
	b.WriteString("Repository name: " + d.RepositoryName + "\n")
	b.WriteString("Branch name: " + d.BranchName + "\n\n")
	b.WriteString("</repository-context>\n")

	if len(d.RecentUserCommits) > 0 {
		b.WriteString("<user-commits>\n")
		b.WriteString("# RECENT USER COMMITS (For reference only, do not copy!):\n")
		for _, c := range d.RecentUserCommits {
			b.WriteString("- " + c + "\n")
		}
		b.WriteString("\n</user-commits>\n")
	}

	if len(d.RecentRepoCommits) > 0 {
		b.WriteString("<recent-commits>\n")
		b.WriteString("# RECENT REPOSITORY COMMITS (For reference only, do not copy!):\n")
		for _, c := range d.RecentRepoCommits {
			b.WriteString("- " + c + "\n")
		}
		b.WriteString("\n</recent-commits>\n")
	}

	b.WriteString("<changes>\n")
	for _, ch := range d.Changes {
		b.WriteString("<original-code>\n")
		b.WriteString("# ORIGINAL CODE:\n")
		b.WriteString(ch.OriginalCode)
		b.WriteString("\n</original-code>\n")

		b.WriteString("<code-changes>\n")
		b.WriteString("# CODE CHANGES:\n")
		b.WriteString("```diff\n")
		b.WriteString(strings.TrimRight(ch.Diff, "\n"))
		b.WriteString("\n```\n")
		b.WriteString("</code-changes>\n")
	}
	b.WriteString("\n</changes>\n")

	b.WriteString("<reminder>\n")
	b.WriteString("Now generate a commit messages that describe the CODE CHANGES.\n")
	b.WriteString("DO NOT COPY commits from RECENT COMMITS, but use it as reference for the commit style.\n")
	b.WriteString("ONLY return a single markdown code block, NO OTHER PROSE!\n")
	b.WriteString("```text\ncommit message goes here\n```\n")
	b.WriteString("</reminder>\n")

	b.WriteString("<custom-instructions>\n")
	if strings.TrimSpace(d.CustomInstructions) != "" {
		b.WriteString(strings.TrimRight(d.CustomInstructions, "\n"))
		b.WriteString("\n")
	}
	b.WriteString("\n</custom-instructions>\n")

	return b.String()
}

func ToOpenAIMessages(vs []VSCodeMessage) []OpenAIMessage {
	out := make([]OpenAIMessage, 0, len(vs))
	for _, m := range vs {
		role := "user"
		if m.Role == 0 {
			role = "system"
		}
		var sb strings.Builder
		for _, p := range m.Content {
			if p.Type == 1 {
				sb.WriteString(p.Text)
			}
		}
		out = append(out, OpenAIMessage{Role: role, Content: sb.String()})
	}
	return out
}

var reTextBlock = regexp.MustCompile("(?ms)^```(?:\\w+)?\\s*([\\s\\S]+?)\\s*```$")

// Returns (contentToPrint, okExactOneTextBlock)
func ExtractOneTextCodeBlock(s string) (string, bool) {
	s = strings.TrimSpace(s)
	// Try to find the block
	m := reTextBlock.FindStringSubmatch(s)
	if len(m) == 2 {
		return strings.TrimSpace(m[1]), true
	}
	// Fallback: if the whole message is code but without blocks (unlikely but possible),
	// or if there's prose.
	// Current caller behavior: if !ok, it prints warning and usage raw s.
	return s, false
}

func DebugPreview(vs []VSCodeMessage) string {
	// helpful if you want quick peek
	return fmt.Sprintf("messages=%d", len(vs))
}
