package analyzer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	conventionalRE = regexp.MustCompile(`^([a-z]+)(?:\(([^)]+)\))?!?:\s+.+`)
	ticketRE       = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-\d+\b`)
)

// CommitStyle captures recurring conventions found in recent repository commits.
type CommitStyle struct {
	TotalCommits       int      `json:"total_commits"`
	ConventionalCount  int      `json:"conventional_count"`
	AverageSubjectSize int      `json:"average_subject_size"`
	Types              []string `json:"types,omitempty"`
	Scopes             []string `json:"scopes,omitempty"`
	UsesEmoji          bool     `json:"uses_emoji"`
	EmojiPlacement     string   `json:"emoji_placement,omitempty"`
	Emojis             []string `json:"emojis,omitempty"`
	UsesTickets        bool     `json:"uses_tickets"`
	TicketExamples     []string `json:"ticket_examples,omitempty"`
}

// AnalyzeCommitStyle infers lightweight, safe style hints from commit subjects.
func AnalyzeCommitStyle(commits []string) CommitStyle {
	style := CommitStyle{TotalCommits: len(commits)}
	if len(commits) == 0 {
		return style
	}

	typeCounts := map[string]int{}
	scopeCounts := map[string]int{}
	emojiCounts := map[string]int{}
	ticketCounts := map[string]int{}
	emojiPrefix := 0
	emojiSuffix := 0
	subjectRunes := 0

	for _, raw := range commits {
		subject := strings.TrimSpace(raw)
		if subject == "" {
			continue
		}
		subjectRunes += utf8.RuneCountInString(subject)

		if m := conventionalRE.FindStringSubmatch(subject); len(m) == 3 {
			style.ConventionalCount++
			typeCounts[m[1]]++
			if strings.TrimSpace(m[2]) != "" {
				scopeCounts[m[2]]++
			}
		}

		if emoji, placement := detectEmoji(subject); emoji != "" {
			emojiCounts[emoji]++
			switch placement {
			case "prefix":
				emojiPrefix++
			case "suffix":
				emojiSuffix++
			}
		}

		for _, ticket := range ticketRE.FindAllString(subject, -1) {
			ticketCounts[ticket]++
		}
	}

	style.AverageSubjectSize = subjectRunes / len(commits)
	style.Types = topKeys(typeCounts, 5)
	style.Scopes = topKeys(scopeCounts, 5)
	style.Emojis = topKeys(emojiCounts, 5)
	style.TicketExamples = topKeys(ticketCounts, 3)
	style.UsesEmoji = len(style.Emojis) > 0 && (emojiPrefix+emojiSuffix)*2 >= len(commits)
	if emojiSuffix > emojiPrefix {
		style.EmojiPlacement = "suffix"
	} else if emojiPrefix > 0 {
		style.EmojiPlacement = "prefix"
	}
	style.UsesTickets = len(style.TicketExamples) > 0
	return style
}

// Guidance renders prompt guidance. It never asks the model to invent ticket IDs.
func (s CommitStyle) Guidance() string {
	if s.TotalCommits == 0 {
		return ""
	}

	var lines []string
	if s.ConventionalCount*2 >= s.TotalCommits && len(s.Types) > 0 {
		lines = append(lines, fmt.Sprintf("- Recent commits mostly use Conventional Commits; prefer types: %s.", strings.Join(s.Types, ", ")))
	}
	if len(s.Scopes) > 0 {
		lines = append(lines, fmt.Sprintf("- Common scopes observed: %s. Use a scope only when it fits the changed files.", strings.Join(s.Scopes, ", ")))
	}
	if s.UsesEmoji && len(s.Emojis) > 0 {
		placement := s.EmojiPlacement
		if placement == "" {
			placement = "near the subject"
		}
		lines = append(lines, fmt.Sprintf("- Emoji style observed: %s placement, common emoji: %s. Use emoji only if it matches the repository style.", placement, strings.Join(s.Emojis, ", ")))
	}
	if s.UsesTickets && len(s.TicketExamples) > 0 {
		lines = append(lines, fmt.Sprintf("- Ticket references observed: %s. Include a ticket only if it already appears in branch name, staged changes, or user instructions; do not invent IDs.", strings.Join(s.TicketExamples, ", ")))
	}
	if s.AverageSubjectSize > 0 {
		lines = append(lines, fmt.Sprintf("- Recent subject length averages about %d characters; keep the subject similarly concise.", s.AverageSubjectSize))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func detectEmoji(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	r, size := utf8.DecodeRuneInString(s)
	if isEmojiRune(r) {
		return s[:size], "prefix"
	}
	r, size = utf8.DecodeLastRuneInString(s)
	if isEmojiRune(r) {
		return s[len(s)-size:], "suffix"
	}
	return "", ""
}

func isEmojiRune(r rune) bool {
	return (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF)
}

func topKeys(counts map[string]int, limit int) []string {
	type item struct {
		key   string
		count int
	}
	items := make([]item, 0, len(counts))
	for key, count := range counts {
		if strings.TrimSpace(key) == "" {
			continue
		}
		items = append(items, item{key: key, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].key < items[j].key
		}
		return items[i].count > items[j].count
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.key
	}
	return out
}
