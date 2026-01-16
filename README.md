# CommitGen

**CommitGen** is an intelligent CLI tool that leverages AI to generate semantic, context-aware git commit messages. It streamlines your workflow by analyzing staged changes and proposing messages that adhere to industry standards (like Conventional Commits).

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8.svg)

## Features

- **AI-Powered Generation**: Uses OpenAI (or compatible providers) to understand code logic and generate meaningful descriptions.
- **Interactive Configuration**: Easily manage settings via a beautiful terminal UI (`commitgen config`).
- **Conventional Commits**: Built-in support for enforcing conventional commit formats (`feat:`, `fix:`, `chore:`, etc.).
- **Smart Token Optimization**:
  - Automatically ignores lockfiles and large assets to save costs.
  - Truncates oversized files while preserving context.
  - Customizable ignore patterns via configuration.
- **Context Aware**: Analyzes recent commit history to maintain consistency with your project's style.

## Installation

### From Source

Ensure you have Go 1.25+ installed.

```bash
# Clone the repository
git clone https://github.com/hoanghonghuy/commitgen.git
cd commitgen

# Install
./install.sh
# OR
go install ./cmd/commitgen
```

Ensure your `$GOPATH/bin` is in your system `PATH`.

## Configuration

Before using, you need to configure your AI provider settings. You can do this interactively:

```bash
commitgen config
```

This will open a UI where you can set:
- **Base URL**: Your AI provider endpoint (e.g., `https://api.openai.com/v1`).
- **API Key**: Your API secret key.
- **Model**: The model to use (e.g., `gpt-4o`, `gpt-3.5-turbo`).
- **Preferences**: 
    - Toggle **Conventional Commits**.
    - Set **Temperature** (creativity).
    - Manage **Ignored Files**.

Configuration is saved to `~/.commitgen.json`.

## Usage

### Generate a Commit Message

Stage your changes and run:

```bash
git add .
commitgen
```

The tool will:
1. Analyze your staged changes.
2. Generate a proposed commit message.
3. Allow you to **Apply**, **Edit**, or **Regenerate** the message.

### CLI Flags

You can override configuration per run:

```bash
# Force specific model and conventional format
commitgen -model gpt-4 -conventional

# See all options
commitgen -h
```

## Contributing

Contributions are welcome! Please open an issue or submit a pull request for any improvements.

1. Fork the Project
2. Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit your Changes (`git commit -m 'feat: Add some AmazingFeature'`)
4. Push to the Branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

Distributed under the MIT License. See `LICENSE` for more information.
