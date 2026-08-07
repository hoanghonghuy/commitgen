# CommitGen

**CommitGen** is an intelligent CLI tool that leverages AI to generate semantic, context-aware git commit messages. It streamlines your workflow by analyzing staged changes and proposing messages that adhere to industry standards (like Conventional Commits).

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8.svg)

## Features

- **AI-Powered Generation**: Uses OpenAI, Anthropic, Gemini, or Ollama (local or [Ollama Cloud](https://ollama.com/cloud)) to understand code logic and generate meaningful descriptions.
- **Interactive Configuration**: Easily manage settings via a beautiful terminal UI (`commitgen config`).
- **Conventional Commits**: Built-in support for enforcing conventional commit formats (`feat:`, `fix:`, `chore:`, etc.).
- **Smart Token Optimization**:
  - Automatically ignores lockfiles and large assets to save costs.
  - **Summarization**: Truncates oversized files while preserving context (e.g., collapsing Go function bodies).
  - Customizable ignore patterns via configuration.
- **Context Aware**: Analyzes recent commit history to maintain consistency with your project's style.
- **Team Style Learning**: Infers common commit types, scopes, emoji style, ticket references, and subject length from recent commits.
- **Git Commit Templates**: Detects configured `commit.template` files and guides AI output to match them.
- **Code Review TUI**: Quick scan and full review of staged changes (`commitgen review`).
- **Git Hooks**: Install a `prepare-commit-msg` generator hook (`commitgen install-hook`) or `commit-msg` validation hook (`commitgen install-msg-hook`).
- **Non-interactive Mode**: `--print` / `--dry-run` for scripts and CI.
- **Multi-candidate**: Generate several message options (`--count N`) and pick in the TUI.
- **Streaming**: Token-by-token display in the TUI for single-message generation (`--count 1`, all streaming-capable providers).
- **Per-repo Config**: `.commitgen.json` in the repo overlays `~/.commitgen.json`.
- **Commit Validation**: Opt-in via `.commitgen-rules.json` or `rules_file` in config.
- **i18n**: UI in English, Vietnamese, Japanese, Chinese (`--locale` / `COMMITGEN_LOCALE`).
- **Connectivity**: `commitgen ping` and `commitgen models` to verify provider setup.
- **PR draft**: `commitgen pr` generates title + description from merge-base vs HEAD (lazycommit-style).
- **Comprehensive Logging**: Structured logging with configurable levels and outputs for debugging and monitoring.

## Commands

| Command | Description |
|---------|-------------|
| `commitgen` / `suggest` | Generate a commit message (default TUI) |
| `commitgen review` | AI code review of staged changes |
| `commitgen pr` | Generate PR title + description from merge-base |
| `commitgen config` | Interactive settings editor |
| `commitgen config show` | Print saved config (secrets masked) |
| `commitgen config path` | Print config file path |
| `commitgen install-hook` | Install `prepare-commit-msg` hook |
| `commitgen uninstall-hook` | Remove `prepare-commit-msg` hook (restores `.bak` if present) |
| `commitgen install-msg-hook` | Install `commit-msg` validation hook |
| `commitgen uninstall-msg-hook` | Remove `commit-msg` validation hook (restores `.bak` if present) |
| `commitgen ping` | Test provider connectivity |
| `commitgen models` | List models (OpenAI-compatible / Ollama) |
| `commitgen style` | Print learned commit style from recent history (`--json` for scripts) |
| `commitgen validate-msg --file MSG` | Validate an existing commit message file for hooks/CI |
| `commitgen dump-prompt` | Export the AI prompt as JSON (debug) |

Common flags: `--print`, `--dry-run`, `--amend`, `--count`, `--timeout`, `--locale`, `--repo`, `--config`, `--base` (for `pr`).

### TUI keys (suggest / review)

| Key | Action |
|-----|--------|
| `↑` / `↓` (or `k` / `j`) | Move selection |
| `Enter` | Confirm selected action |
| `y` | Copy message / report to clipboard |
| `Esc` | Cancel in-flight generate/analyze; leave choose without selecting; abort regen guidance |
| `1`–`4` | Jump to review menu item (optional hotkeys) |
| `Ctrl+C` | Quit |
| `PgUp` / `PgDn` | Scroll long content |

Config form: abort step 2 (Esc / cancel) returns to provider selection. Set `ACCESSIBLE=1` or `TERM=dumb` for huh accessible mode.

### Pull request draft

```bash
# On a feature branch with commits ahead of main:
commitgen pr
commitgen pr --base develop
commitgen pr --base main --locale vi
```

Output is markdown (`# title` + Summary / Test plan) suitable for copying into GitHub/GitLab or scripting with `gh pr create`.

## Project Structure

The project is organized into several packages:

- `cmd/commitgen/`: Main entry point for the CLI application.
- `internal/ai/`: Common interface for AI providers.
- `internal/vscodeprompt/`: Core engine for building VS Code-style prompts and source code summarization.
- `internal/gitx/`: Git utilities for diffing, logging, and committing.
- `internal/app/`: Main application logic, TUI, and Git hook management.
- `internal/config/`: User configuration management (`~/.commitgen.json`).
- `internal/logger/`: Structured logging system with multiple output options.

## Installation & Build

Ensure you have Go 1.25+ installed.

### Build from Source

```bash
# Clone the repository
git clone https://github.com/hoanghonghuy/commitgen.git
cd commitgen

# Build the executable
go build -o commitgen.exe ./cmd/commitgen
```

### Install to GOPATH

```bash
go install ./cmd/commitgen
```

## Configuration

Before using, configure your AI provider interactively:

```bash
commitgen config
```

The form is **two-step**: (1) choose provider, (2) model + API key for that provider. Base URL appears only for **OpenAI Compatible**.

Configuration is saved to `~/.commitgen.json` by default:

| Provider id | Base URL | API key slot (`api_keys`) |
|-------------|----------|---------------------------|
| `openai` | Fixed `https://api.openai.com/v1` | `openai` |
| `openrouter` | Fixed `https://openrouter.ai/api/v1` | `openrouter` |
| `compatible` | Editable (`compatible_base_url`) | `compatible` |
| `ollama` | Fixed `http://localhost:11434` | `ollama` |
| `ollama-cloud` | Fixed `https://ollama.com` | `ollama` (shared with local) |
| `anthropic` | Anthropic default | `anthropic` |
| `gemini` | Gemini default | `gemini` |

Example schema:

```json
{
  "provider": "openrouter",
  "model": "anthropic/claude-sonnet-4",
  "api_keys": {
    "openai": "...",
    "openrouter": "...",
    "ollama": "..."
  }
}
```

Legacy `api_key` / `base_url` / `anthropic_key` / `gemini_key` are **migrated automatically on load** and rewritten to the new schema.

Other settings:
- **Preferences**: Conventional Commits, Summarization, Ignored Files.
- **Advanced** (edit JSON): `timeout_seconds`, `prompt_template_file`, `rules_file`.
- **Per-repo**: Place `.commitgen.json` in the repository root to override global settings. Use `commitgen config --config .commitgen.json` to edit repo-local settings.
- **Validation**: Place `.commitgen-rules.json` in the repo root (auto-discovered) or set `rules_file` in config.
- **Logging**: Log level, output destination, and log file path.

`commitgen config` loads merged settings (global + repo overlay) but **saves to `~/.commitgen.json`** unless you pass `--config`. Leave the API key empty (or `********`) to keep the existing key for that provider.

### Environment variables

| Variable | Purpose |
|----------|---------|
| `COMMITGEN_API_KEY` | Overrides the API key for the **active** provider |
| `OLLAMA_API_KEY` | Ollama key ([create at ollama.com/settings/keys](https://ollama.com/settings/keys)) — preferred over `COMMITGEN_API_KEY` for `ollama` / `ollama-cloud` |
| `COMMITGEN_PROVIDER` | Provider id (`openai`, `openrouter`, `compatible`, `ollama`, `ollama-cloud`, `anthropic`, `gemini`) |
| `COMMITGEN_BASE_URL` | Custom base URL — only applied when `provider=compatible` |
| `COMMITGEN_MODEL` | Model name |
| `COMMITGEN_LOCALE` | UI language (`en`, `vi`, `ja`, `zh`) |

Verify with `commitgen ping`.

### Locale

```bash
commitgen --locale vi
commitgen --locale auto   # detect from LANG / LC_ALL
export COMMITGEN_LOCALE=ja
```

Set `locale` to `auto` in `~/.commitgen.json` to follow the system locale on each run.

CommitGen includes comprehensive logging to help debug issues:

```bash
# Set log level via flag
commitgen --log-level debug

# Set log output destination
commitgen --log-output both  # logs to both stderr and file

# Custom log file path
commitgen --log-file /path/to/custom.log

# Or configure via environment variables
export COMMITGEN_LOG_LEVEL=debug
export COMMITGEN_LOG_OUTPUT=both
```

**Default log location**: `~/.commitgen/commitgen.log`

When errors occur in the TUI (alternate screen), they are automatically logged to the file and displayed after the TUI exits, so you won't lose error information.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request for any improvements.

1. Fork the Project
2. Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit your Changes (`git commit -m 'feat: Add some AmazingFeature'`)
4. Push to the Branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## Troubleshooting

### Windows: Application Control Policy Error
If you encounter the error `Program 'commitgen.exe' failed to run: An Application Control policy has blocked this file` on Windows, it is likely because the pre-compiled binary is not digitally signed.

**Solution**: Rebuild the binary locally from source. This will create a binary that is trusted by your local system.

```powershell
# Remove the existing binary
Remove-Item commitgen.exe

# Rebuild from source
go build -o commitgen.exe ./cmd/commitgen
```

## License

Distributed under the MIT License. See `LICENSE` for more information.
