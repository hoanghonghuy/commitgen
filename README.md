# CommitGen

**CommitGen** is an intelligent CLI tool that leverages AI to generate semantic, context-aware git commit messages. It streamlines your workflow by analyzing staged changes and proposing messages that adhere to industry standards (like Conventional Commits).

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8.svg)

## Features

- **AI-Powered Generation**: Uses OpenAI, Anthropic, Gemini, or Ollama to understand code logic and generate meaningful descriptions.
- **Interactive Configuration**: Easily manage settings via a beautiful terminal UI (`commitgen config`).
- **Conventional Commits**: Built-in support for enforcing conventional commit formats (`feat:`, `fix:`, `chore:`, etc.).
- **Smart Token Optimization**:
  - Automatically ignores lockfiles and large assets to save costs.
  - **Summarization**: Truncates oversized files while preserving context (e.g., collapsing Go function bodies).
  - Customizable ignore patterns via configuration.
- **Context Aware**: Analyzes recent commit history to maintain consistency with your project's style.
- **Comprehensive Logging**: Structured logging with configurable levels and outputs for debugging and monitoring.

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

Configuration is saved to `~/.commitgen.json` and includes:
- **Provider**: `openai`, `anthropic`, `gemini`, or `ollama`.
- **Base URL**: Your AI provider endpoint.
- **API Key**: Your API secret key.
- **Model**: The model to use (e.g., `gpt-4o`, `claude-3-5-sonnet`, `gemini-1.5-pro`).
- **Preferences**: Toggle Conventional Commits, Summarization, and manage Ignored Files.
- **Logging**: Configure log level (debug, info, warn, error), output destination (stderr, file, both), and log file path.

## Logging

CommitGen includes comprehensive logging to help debug issues:

```bash
# Set log level via flag
commitgen --log-level debug

# Set log output destination
commitgen --log-output both  # logs to both stderr and file

# Custom log file path
commitgen --log-file /path/to/custom.log

# Or configure via environment variables
export COMMITAI_LOG_LEVEL=debug
export COMMITAI_LOG_OUTPUT=both
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
