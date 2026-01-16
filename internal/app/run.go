package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"commitgen/internal/config"
	"commitgen/internal/gitx"
	"commitgen/internal/openai"
	"commitgen/internal/vscodeprompt"

	"github.com/briandowns/spinner"
)

type Config struct {
	Command string

	RepoArg string

	BaseURL string
	APIKey  string
	Model   string

	RecentN   int
	MaxFiles  int
	Summarize bool

	Temperature float64
	Timeout     time.Duration // assigned in main, not used here directly

	DumpOutPath string

	InstructionsPath string

	// Config management
	ConfigPath string
	SaveConfig bool

	// Enhancements
	Conventional bool
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Command == "config" {
		return runConfig(cfg)
	}

	repoRoot, err := gitx.ResolveRepoRoot(cfg.RepoArg)
	if err != nil {
		return err
	}

	customInstructions := ""
	if strings.TrimSpace(cfg.InstructionsPath) != "" {
		b, err := os.ReadFile(cfg.InstructionsPath)
		if err != nil {
			return fmt.Errorf("read instructions file: %w", err)
		}
		customInstructions = string(b)
	}

	data, err := buildPromptData(repoRoot, cfg.RecentN, cfg.MaxFiles, cfg.Summarize, customInstructions)
	if err != nil {
		return err
	}

	vscodeMsgs := vscodeprompt.BuildVSCodeMessages(data)

	switch cfg.Command {
	case "dump-prompt":
		return dumpPrompt(vscodeMsgs, cfg.DumpOutPath)

	case "suggest":
		if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" {
			return errors.New("missing base-url/model. Set flags or env COMMITAI_BASE_URL / COMMITAI_MODEL")
		}

		client := openai.New(openai.Config{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		})

		oaiMsgs := vscodeprompt.ToOpenAIMessages(vscodeMsgs)

		// Interactive Loop
		for {
			// Spinner
			s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
			s.Suffix = " Generating commit message..."
			s.Start()

			// Add Conventional Commits instruction if requested
			if cfg.Conventional {
				// Append a USER message at the end to enforce specific format, overriding previous context if necessary.
				reminderMsg := vscodeprompt.OpenAIMessage{
					Role:    "user",
					Content: "CRITICAL INSTRUCTION: You must strictly follow the Conventional Commits specification (e.g. 'feat: add spinner', 'fix: resolve bug').\nDo not just describe the change; prefix it with the type.",
				}
				oaiMsgs = append(oaiMsgs, reminderMsg)
			}

			out, err := client.Chat(ctx, openai.ChatRequest{
				Messages:    oaiMsgs,
				Temperature: cfg.Temperature,
			})
			s.Stop() // Stop spinner

			if err != nil {
				return err
			}

			commitMsg, ok := vscodeprompt.ExtractOneTextCodeBlock(out)
			if !ok {
				fmt.Fprintln(os.Stderr, "Warning: model formatting issue (raw output shown below)")
				commitMsg = out
			}

			// Interactive Confirmation
		ConfirmStep:
			action, err := confirmCommitInteractive(commitMsg)
			if err != nil {
				return err
			}

			switch action {
			case ActionCommit:
				return gitx.Commit(repoRoot, commitMsg)

			case ActionEdit:
				newMsg, err := editCommitMessageInteractive(commitMsg)
				if err != nil {
					return err
				}
				commitMsg = newMsg
				// Loop back to confirmation without regenerating
				// We need to bypass generation step.
				// Refactor loop: Generation -> [Confirmation Loop -> (Commit|Regen|Edit)]
				// But currently it's one big loop.
				// Easiest fix: use a label or flag, OR just go to top of loop but skip generic call?
				// Better: Nested loop.
				// Outer Loop (Generation)
				//   Generate
				//   Inner Loop (Confirmation)
				//     Show
				//     Ask
				//     Handle: Edit -> Update msg -> Continue Inner Loop
				//             Regen -> Continue Outer Loop
				//             Commit -> Return

				// Let's refactor the loop structure.
				goto ConfirmStep

			case ActionRegenerate:
				fmt.Println("Regenerating...")
				continue
			case ActionCancel:
				fmt.Println("Cancelled.")
				return nil
			}
		}

	default:
		return fmt.Errorf("unknown -cmd=%s (use suggest | dump-prompt | config)", cfg.Command)
	}
}

func runConfig(cfg Config) error {
	// If flags are provided (e.g. -save or -api-key), we assume non-interactive mode (or mixed).
	// But the user requested upgrade.
	// Let's fallback to interactive if no specific property flags were set?
	// Easier: Just always launching interactive if "config" is called, UNLESS maybe just viewing?
	// For now, let's launch interactive form, then save.

	newCfg, ok, err := runConfigInteractive(cfg)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Operation cancelled.")
		return nil
	}

	// Always save in interactive mode
	fileCfg := config.FileConfig{
		BaseURL: newCfg.BaseURL,
		APIKey:  newCfg.APIKey,
		Model:   newCfg.Model,
	}
	// Warning: We need to know where to save. newCfg has ConfigPath.
	if err := config.Save(fileCfg, cfg.ConfigPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("\nConfiguration saved to %s\n", cfg.ConfigPath)
	return nil
}

func maskKey(k string) string {
	if len(k) < 8 {
		return "*****"
	}
	return k[:4] + "..." + k[len(k)-4:]
}

func dumpPrompt(msgs []vscodeprompt.VSCodeMessage, outPath string) error {
	if strings.TrimSpace(outPath) == "" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(msgs)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(msgs); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}

func buildPromptData(repoRoot string, recentN, maxFiles int, summarize bool, customInstructions string) (vscodeprompt.Data, error) {
	repoName := gitx.RepoNameFromRoot(repoRoot)

	branch, _ := gitx.CurrentBranch(repoRoot)
	userEmail, _ := gitx.GitConfig(repoRoot, "user.email")

	userCommits, _ := gitx.RecentCommitsByAuthor(repoRoot, recentN, userEmail)
	repoCommits, _ := gitx.RecentCommits(repoRoot, recentN)

	changes, err := gitx.StagedChanges(repoRoot, maxFiles)
	if err != nil {
		return vscodeprompt.Data{}, err
	}
	if len(changes) == 0 {
		return vscodeprompt.Data{}, errors.New("no staged changes. Run: git add -A")
	}

	// Build attachments like VSCode prompt: <attachment ... isSummarized="true"> with numbered lines & filepath comment
	att := make([]vscodeprompt.Change, 0, len(changes))
	for _, ch := range changes {
		orig, _ := gitx.OriginalFileAtHEAD(repoRoot, ch.Path)
		if strings.TrimSpace(orig) == "" {
			orig, _ = gitx.ReadWorkingTreeFile(repoRoot, ch.Path)
		}
		attachment := vscodeprompt.BuildAttachment(repoRoot, ch.Path, orig, summarize)
		att = append(att, vscodeprompt.Change{
			Path:         ch.Path,
			Diff:         ch.Diff,
			OriginalCode: attachment,
		})
	}

	return vscodeprompt.Data{
		RepositoryName:       repoName,
		BranchName:           branch,
		RecentUserCommits:    userCommits,
		RecentRepoCommits:    repoCommits,
		Changes:              att,
		CustomInstructions:   customInstructions, // inserted into <custom-instructions>
		SummarizeAttachments: summarize,
	}, nil
}
