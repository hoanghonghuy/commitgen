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
- **Code Review TUI**: Quick scan and full review of staged changes (`commitgen review`).
- **Git Hook**: Install a `prepare-commit-msg` hook (`commitgen install-hook`) — always headless, writes the message via `--print`.
- **Non-interactive Mode**: `--print` / `--dry-run` for scripts and CI.
- **Multi-candidate**: Generate several message options (`--count N`) and pick in the TUI.
- **Streaming**: Token-by-token display in the TUI (all providers).
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
| `commitgen uninstall-hook` | Remove hook (restores `.bak` if present) |
| `commitgen ping` | Test provider connectivity |
| `commitgen models` | List models (OpenAI-compatible / Ollama) |
| `commitgen dump-prompt` | Export the AI prompt as JSON (debug) |

Common flags: `--print`, `--dry-run`, `--amend`, `--count`, `--timeout`, `--locale`, `--repo`, `--config`, `--base` (for `pr`).

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

Before using, you need to configure your AI provider settings. You can do this interactively:

```bash
commitgen config
```

Configuration is saved to `~/.commitgen.json` by default and includes:
- **Provider**: `openai`, `anthropic`, `gemini`, or `ollama` (local `localhost:11434` or cloud `https://ollama.com` + API key).
- **Base URL**: Your AI provider endpoint.
- **API Key**: Your API secret key (OpenAI-compatible providers and Ollama Cloud).
- **Model**: The model to use (e.g., `gpt-4o`, `claude-3-5-sonnet`, `gemini-1.5-pro`, `deepseek-v4-pro`).
- **Preferences**: Toggle Conventional Commits, Summarization, and manage Ignored Files.
- **Advanced** (edit JSON directly): `timeout_seconds`, `prompt_template_file`, `rules_file`.
- **Per-repo**: Place `.commitgen.json` in the repository root to override global settings. Use `commitgen config --config .commitgen.json` to edit repo-local settings.
- **Validation**: Place `.commitgen-rules.json` in the repo root (auto-discovered) or set `rules_file` in config.
- **Logging**: Configure log level (debug, info, warn, error), output destination (stderr, file, both), and log file path.

`commitgen config` loads merged settings (global + repo overlay) but **saves to `~/.commitgen.json`** unless you pass `--config`. Leave API key fields empty in the form to keep existing secrets.

### Environment variables

| Variable | Purpose |
|----------|---------|
| `COMMITGEN_API_KEY` | API key (OpenAI-compatible providers; fallback for Ollama) |
| `OLLAMA_API_KEY` | Ollama Cloud API key ([create at ollama.com/settings/keys](https://ollama.com/settings/keys)) — preferred over `COMMITGEN_API_KEY` when `provider=ollama` |
| `COMMITGEN_PROVIDER` | `openai`, `ollama`, `anthropic`, `gemini` |
| `COMMITGEN_BASE_URL` | Provider base URL |
| `COMMITGEN_MODEL` | Model name |
| `COMMITGEN_LOCALE` | UI language (`en`, `vi`, `ja`, `zh`) |

For Ollama Cloud, set `base_url` to `https://ollama.com` and provide an API key via config, `OLLAMA_API_KEY`, or `--api-key`. Verify with `commitgen ping`.

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
