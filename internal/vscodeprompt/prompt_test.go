package vscodeprompt

import (
	"testing"
)

func TestExtractOneTextCodeBlock(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOk bool
	}{
		{
			name:   "text block",
			input:  "```text\ncommit message\n```",
			want:   "commit message",
			wantOk: true,
		},
		{
			name:   "markdown block",
			input:  "```markdown\ncommit message\n```",
			want:   "commit message",
			wantOk: true,
		},
		{
			name:   "no lang block",
			input:  "```\ncommit message\n```",
			want:   "commit message",
			wantOk: true,
		},
		{
			name:   "surrounding whitespace",
			input:  "  ```\ncommit message\n```  ",
			want:   "commit message",
			wantOk: true,
		},
		{
			name:   "multiline message",
			input:  "```\nfeat: add something\n\nBody line.\n```",
			want:   "feat: add something\n\nBody line.",
			wantOk: true,
		},
		{
			name:   "prose only",
			input:  "Just some text",
			want:   "Just some text",
			wantOk: false,
		},
		{
			name:   "prose with code",
			input:  "Here is the code:\n```\nfeat: x\n```",
			want:   "feat: x",
			wantOk: true, // Regex should find it now with (?s)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ExtractOneTextCodeBlock(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ExtractOneTextCodeBlock() ok = %v, want %v", ok, tt.wantOk)
			}
			if got != tt.want {
				t.Errorf("ExtractOneTextCodeBlock() got = %q, want %q", got, tt.want)
			}
		})
	}
}
