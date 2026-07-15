# Future Improvements for CommitGen

This document outlines potential improvements and new features for CommitGen that would enhance user experience and functionality.

---

## 🌐 i18n & Localization

### Current State (implemented)

The application ships with `internal/i18n` (en, vi, ja, zh). TUI and CLI errors use the translator with English fallback. Config form (`commitgen config`) labels remain English.

Set locale via `--locale`, `COMMITGEN_LOCALE`, or `locale` in `~/.commitgen.json`.

**Not yet implemented:** `--locale auto` from `$LANG`.

---

## 🚀 Feature Additions

### 3. Commit Message Validation (implemented)

Opt-in validation via `.commitgen-rules.json` (auto-discovered in repo root) or `rules_file` in config. TUI shows validation issues with auto-fix; `--print`/hook path rejects messages with error-level violations.

See `internal/validator/` and example rules in `docs/future-improvements.md` below.

---

### Original proposals (not yet implemented)

The sections below describe **future** ideas. Items marked in the priority matrix as done in the original doc were aspirational — verify against README before assuming they exist.

## 🌐 i18n & Localization (original proposal — largely implemented)

### Current State

The application currently has hard-coded English strings throughout the UI, with only partial support for localized review output via the `review_language` setting.

**Example of hard-coded strings:**
```go
// internal/app/tui.go
styleEditTitle.Render("Edit Commit Message")
styleMsgTitle.Render("Generated Commit Message")
styleActionTitle.Render("Action")

// internal/app/review.go
options := []string{"Copy to clipboard", "Suggest commit message", "Done"}
```

### Problems

1. **Non-English users must read English UI** - Vietnamese, Japanese, Chinese users have to work with English interface
2. **Inconsistent localization** - Review output can be localized but UI cannot
3. **Hard to maintain** - Strings scattered across codebase
4. **No fallback mechanism** - Cannot gracefully handle missing translations

### Proposed Solution

#### 1. Extract All UI Strings

Create a centralized i18n package:

```go
// internal/i18n/i18n.go
package i18n

type Locale string

const (
    LocaleEN Locale = "en"
    LocaleVI Locale = "vi"
    LocaleJA Locale = "ja"
    LocaleZH Locale = "zh"
)

type Translator struct {
    locale   Locale
    messages map[Locale]map[string]string
}

func New(locale Locale) *Translator {
    return &Translator{
        locale:   locale,
        messages: loadMessages(),
    }
}

func (t *Translator) T(key string, args ...interface{}) string {
    msg, ok := t.messages[t.locale][key]
    if !ok {
        // Fallback to English
        msg = t.messages[LocaleEN][key]
    }
    if msg == "" {
        return key // Return key if translation missing
    }
    return fmt.Sprintf(msg, args...)
}
```


#### 2. Message Files Structure

```
internal/i18n/
├── i18n.go
├── en.json
├── vi.json
├── ja.json
└── zh.json
```

**Example: `en.json`**
```json
{
  "tui.title.generated_message": "Generated Commit Message",
  "tui.title.action": "Action",
  "tui.title.edit": "Edit Commit Message",
  "tui.action.commit": "Commit (Apply)",
  "tui.action.regenerate": "Regenerate",
  "tui.action.edit": "Edit",
  "tui.action.cancel": "Cancel",
  "tui.hint.generating": "Generating commit message...",
  "tui.hint.committing": "Committing...",
  "tui.hint.edit_instructions": "Press Esc to finish editing",
  "tui.hint.regen_instructions": "Enter to regenerate, Esc to cancel",
  "tui.state.copied": "Copied to clipboard!",
  "tui.state.success": "Committed successfully!",
  "error.no_staged_changes": "No staged changes. Run: git add -A",
  "error.all_files_ignored": "All staged files were ignored (checked %d files)",
  "error.missing_model": "Missing model. Set flags or env COMMITGEN_MODEL",
  "error.missing_api_key": "Missing API key. Set --api-key flag or env COMMITGEN_API_KEY"
}
```

**Example: `vi.json`**
```json
{
  "tui.title.generated_message": "Commit Message Được Tạo",
  "tui.title.action": "Hành Động",
  "tui.title.edit": "Chỉnh Sửa Commit Message",
  "tui.action.commit": "Commit (Áp dụng)",
  "tui.action.regenerate": "Tạo Lại",
  "tui.action.edit": "Chỉnh Sửa",
  "tui.action.cancel": "Hủy",
  "tui.hint.generating": "Đang tạo commit message...",
  "tui.hint.committing": "Đang commit...",
  "tui.hint.edit_instructions": "Nhấn Esc để hoàn tất chỉnh sửa",
  "tui.hint.regen_instructions": "Enter để tạo lại, Esc để hủy",
  "tui.state.copied": "Đã sao chép vào clipboard!",
  "tui.state.success": "Commit thành công!",
  "error.no_staged_changes": "Không có thay đổi nào được staged. Chạy: git add -A",
  "error.all_files_ignored": "Tất cả file đã bị bỏ qua (đã kiểm tra %d files)",
  "error.missing_model": "Thiếu model. Đặt flag hoặc env COMMITGEN_MODEL",
  "error.missing_api_key": "Thiếu API key. Đặt flag --api-key hoặc env COMMITGEN_API_KEY"
}
```


#### 3. Integration with Existing Code

**Before:**
```go
// internal/app/tui.go
styleEditTitle.Render("Edit Commit Message")
```

**After:**
```go
// internal/app/tui.go
type tuiModel struct {
    // ... existing fields
    i18n *i18n.Translator
}

func newTuiModel(..., locale string) tuiModel {
    return tuiModel{
        // ... existing initialization
        i18n: i18n.New(i18n.Locale(locale)),
    }
}

// Usage in View()
styleEditTitle.Render(m.i18n.T("tui.title.edit"))
```

#### 4. Configuration

Add locale to config:

```json
{
  "provider": "openai",
  "model": "gpt-4o",
  "locale": "vi",
  "review_language": "Vietnamese"
}
```

```go
// Config resolution
cfg.Locale = config.ResolveString(*localeFlag, os.Getenv("COMMITGEN_LOCALE"), fileCfg.Locale, "en")
```

#### 5. Command Line Flag

```bash
# Set locale via flag
commitgen --locale vi

# Set via environment
export COMMITGEN_LOCALE=vi
commitgen

# Auto-detect from system
commitgen --locale auto  # Uses $LANG
```


### Implementation Plan

**Phase 1: Foundation (Week 1)**
- [ ] Create `internal/i18n` package
- [ ] Extract all English strings to `en.json`
- [ ] Implement `Translator` with fallback logic
- [ ] Add locale config field

**Phase 2: Translations (Week 2)**
- [ ] Add Vietnamese translations (`vi.json`)
- [ ] Add Japanese translations (`ja.json`)
- [ ] Add Chinese translations (`zh.json`)
- [ ] Add translation validation tests

**Phase 3: Integration (Week 3)**
- [ ] Update `tui.go` to use translator
- [ ] Update `review.go` to use translator
- [ ] Update error messages to use translator
- [ ] Add locale auto-detection

**Phase 4: Testing & Documentation (Week 4)**
- [ ] Test all locales
- [ ] Add locale switching tests
- [ ] Update README with locale instructions
- [ ] Create translation contribution guide

### Benefits

- ✅ **Better UX for non-English users** - Native language interface
- ✅ **Maintainability** - Centralized string management
- ✅ **Extensibility** - Easy to add new languages
- ✅ **Consistency** - Uniform translation approach
- ✅ **Community contributions** - Users can contribute translations

### Challenges

- ⚠️ **Initial work** - Extracting all strings takes time
- ⚠️ **Translation quality** - Need native speakers for accuracy
- ⚠️ **Bundle size** - Multiple locale files increase binary size (minimal impact)
- ⚠️ **Maintenance** - Need to update translations when adding features

---


## 🚀 Feature Additions

### 1. Git Commit Templates Integration

#### Problem
Users often have `.git/commit_template` or `.gitmessage` files that define their team's commit message structure, but CommitGen ignores these templates.

#### Proposed Solution

**Auto-detect and respect git commit templates:**

```go
// internal/gitx/template.go
package gitx

func GetCommitTemplate(ctx context.Context, repoRoot string) (string, error) {
    // Try repo-specific template first
    templatePath, err := GitConfig(ctx, repoRoot, "commit.template")
    if err == nil && templatePath != "" {
        content, err := os.ReadFile(filepath.Join(repoRoot, templatePath))
        if err == nil {
            return string(content), nil
        }
    }
    
    // Fallback to global template
    globalTemplate, err := GitConfig(ctx, repoRoot, "commit.template")
    if err == nil && globalTemplate != "" {
        content, err := os.ReadFile(globalTemplate)
        if err == nil {
            return string(content), nil
        }
    }
    
    return "", nil
}

func ParseTemplate(template string) TemplateStructure {
    // Parse template to identify:
    // - Subject line format
    // - Body structure
    // - Footer requirements (e.g., Signed-off-by, Co-authored-by)
    return TemplateStructure{
        SubjectFormat: "{{type}}({{scope}}): {{description}}",
        RequiredFooters: []string{"Signed-off-by"},
    }
}
```

**Integration with AI prompt:**

```go
// internal/vscodeprompt/prompt.go
func BuildVSCodeMessages(d Data) []VSCodeMessage {
    systemText := renderTemplate(tmpl, d)
    
    // Add template guidance if present
    if d.CommitTemplate != "" {
        systemText += "\n\nThe repository uses this commit template:\n" + d.CommitTemplate
        systemText += "\nPlease generate a commit message that follows this template structure."
    }
    
    // ... rest of function
}
```


**Usage:**

```bash
# CommitGen automatically detects template
commitgen

# Output respects template format:
# feat(auth): implement JWT token refresh
# 
# - Add refresh token endpoint
# - Implement token rotation logic
# - Add tests for token expiration
#
# Signed-off-by: User Name <user@example.com>
```

**Benefits:**
- ✅ Seamless integration with existing team workflows
- ✅ Respects organizational standards
- ✅ No manual template copying

---

### 2. Team Style Learning

#### Problem
Each team has unique commit message conventions that go beyond conventional commits (emoji usage, ticket references, specific wording patterns).

#### Proposed Solution

**Analyze team commit patterns and adapt:**

```go
// internal/analyzer/style.go
package analyzer

type CommitStyle struct {
    UsesEmoji         bool
    EmojiPlacement    string // "prefix" or "suffix"
    CommonEmojis      map[string]string // feat -> 🎉, fix -> 🐛
    TicketPattern     string // e.g., "JIRA-\d+"
    TicketPlacement   string // "prefix", "suffix", "footer"
    AverageLength     int
    UsesImperativeMood bool
    CommonPhrases     []string
}

func AnalyzeRepoStyle(commits []string) CommitStyle {
    style := CommitStyle{
        CommonEmojis: make(map[string]string),
    }
    
    emojiCount := 0
    for _, commit := range commits {
        // Detect emoji usage
        if hasEmoji(commit) {
            emojiCount++
            emoji, pos := extractEmoji(commit)
            style.CommonEmojis[detectType(commit)] = emoji
            style.EmojiPlacement = pos
        }
        
        // Detect ticket pattern
        if ticket := extractTicket(commit); ticket != "" {
            style.TicketPattern = inferPattern(ticket)
        }
        
        // Analyze length
        style.AverageLength += len(commit)
    }
    
    style.UsesEmoji = float64(emojiCount)/float64(len(commits)) > 0.5
    style.AverageLength /= len(commits)
    
    return style
}
```


**Integration with prompt:**

```go
// Build style guidance for AI
func buildStyleGuidance(style CommitStyle) string {
    var guidance strings.Builder
    
    guidance.WriteString("The team follows these conventions:\n")
    
    if style.UsesEmoji {
        guidance.WriteString(fmt.Sprintf("- Use emojis at the %s\n", style.EmojiPlacement))
        guidance.WriteString("- Common emojis: ")
        for typ, emoji := range style.CommonEmojis {
            guidance.WriteString(fmt.Sprintf("%s for %s, ", emoji, typ))
        }
        guidance.WriteString("\n")
    }
    
    if style.TicketPattern != "" {
        guidance.WriteString(fmt.Sprintf("- Include ticket reference matching pattern: %s\n", style.TicketPattern))
    }
    
    guidance.WriteString(fmt.Sprintf("- Keep commit messages around %d characters\n", style.AverageLength))
    
    return guidance.String()
}
```

**Example output:**

```bash
# Input: Team uses emojis and Jira tickets
# Learned style: "🎉 feat(scope): description [JIRA-123]"

commitgen
# Output:
# 🎉 feat(auth): implement OAuth2 authorization [JIRA-1234]
```

**Benefits:**
- ✅ Zero-configuration style matching
- ✅ Adapts to team culture
- ✅ Reduces manual editing

---

### 3. Commit Message Validation (original proposal — implemented)

#### Problem
Generated messages might not meet all project requirements (length limits, required keywords, prohibited patterns).

#### Implemented

See `internal/validator/validator.go` and `.commitgen-rules.json` in the repo root. Validation is **opt-in** (not enabled by default without a rules file).

#### Original proposed solution (reference)

**Pre-commit validation with auto-fix suggestions:**

```go
// internal/validator/validator.go
package validator

type Rule interface {
    Validate(msg string) []Issue
    AutoFix(msg string) (string, bool) // returns (fixed, canAutoFix)
}

type Issue struct {
    Level   string // "error", "warning"
    Message string
    Line    int
    Column  int
    Rule    string
}

type Validator struct {
    rules []Rule
}

// Built-in rules
type SubjectLengthRule struct {
    MaxLength int
}

func (r *SubjectLengthRule) Validate(msg string) []Issue {
    lines := strings.Split(msg, "\n")
    if len(lines[0]) > r.MaxLength {
        return []Issue{{
            Level: "error",
            Message: fmt.Sprintf("Subject line too long: %d > %d", len(lines[0]), r.MaxLength),
            Rule: "subject-length",
        }}
    }
    return nil
}
```


**Configuration via `.commitgen-rules.json`:**

```json
{
  "rules": {
    "subject-length": {
      "enabled": true,
      "max": 50
    },
    "body-line-length": {
      "enabled": true,
      "max": 72
    },
    "required-footers": {
      "enabled": true,
      "footers": ["Signed-off-by"]
    },
    "prohibited-words": {
      "enabled": true,
      "words": ["WIP", "temp", "debug"]
    },
    "required-patterns": {
      "enabled": true,
      "patterns": ["^(feat|fix|docs|style|refactor|test|chore)(\\(.+\\))?: .+"]
    }
  }
}
```

**Integration with TUI:**

```go
// Before committing, validate
issues := validator.Validate(m.commitMsg)
if len(issues) > 0 {
    m.state = stateValidationFailed
    m.validationIssues = issues
    return m, nil
}
```

**TUI Validation State:**

```
❌ Validation Failed

Issues:
  • [ERROR] Subject line too long: 65 > 50
    Line 1: "feat(auth): implement very complex OAuth2 authorization flow with multiple providers"
    
  • [WARNING] Missing footer: Signed-off-by
    Add: Signed-off-by: John Doe <john@example.com>

Actions:
  > Auto-fix (recommended)
  > Edit manually
  > Ignore warnings
  > Cancel
```

**Benefits:**
- ✅ Catch issues before commit
- ✅ Auto-fix common problems
- ✅ Enforce team standards

---


### 4. Interactive Fix Suggestions (AI-Powered Code Review)

#### Problem
Review mode identifies issues but doesn't help fix them. Users must manually address each issue.

#### Proposed Solution

**AI suggests and applies fixes:**

```go
// internal/fixer/fixer.go
package fixer

type FixSuggestion struct {
    Issue       string
    Suggestion  string
    Confidence  float64 // 0.0 to 1.0
    AutoApply   bool
    Changes     []FileChange
}

type FileChange struct {
    Path      string
    OldCode   string
    NewCode   string
    StartLine int
    EndLine   int
}

func SuggestFixes(ctx context.Context, provider ai.Provider, review string, changes []Change) ([]FixSuggestion, error) {
    prompt := buildFixPrompt(review, changes)
    response, err := provider.Generate(ctx, prompt, 0.3) // Low temp for deterministic fixes
    if err != nil {
        return nil, err
    }
    return parseFixes(response), nil
}
```

**Example interaction:**

```
📝 Code Review Summary

Issues Found:
  1. Missing error handling in AuthHandler (line 45)
  2. Hardcoded API endpoint should use config
  3. Function 'processData' is too complex (cyclomatic complexity: 15)

💡 Fix Suggestions

[1] Add error handling
  Confidence: 95%
  File: internal/auth/handler.go
  
  - user, _ := getUser(id)
  + user, err := getUser(id)
  + if err != nil {
  +     return fmt.Errorf("get user: %w", err)
  + }
  
  Apply this fix? [Y/n/skip]

[2] Extract config value
  Confidence: 90%
  File: internal/client/api.go
  
  - url := "https://api.example.com/v1"
  + url := cfg.APIBaseURL
  
  Apply this fix? [Y/n/skip]

[3] Refactor complex function
  Confidence: 70%
  Suggested approach: Extract 3 helper functions
  
  Would you like to see the detailed refactoring plan? [y/N]
```


**Smart commit splitting:**

```
📊 Commit Analysis

Your staged changes include:
  • 3 new features
  • 2 bug fixes
  • 1 refactoring

💡 Recommendation: Split into 6 smaller commits

Suggested commits:

[1] feat(auth): add JWT token refresh
    Files: internal/auth/token.go, internal/auth/refresh.go
    +125 -15 lines
    
[2] feat(api): implement user profile endpoint
    Files: internal/api/profile.go, internal/api/routes.go
    +89 -3 lines

[3] fix(db): resolve connection pool leak
    Files: internal/db/pool.go
    +12 -8 lines

... (3 more)

Actions:
  > Auto-split and commit
  > Show detailed plan
  > Commit as-is
  > Cancel
```

**Benefits:**
- ✅ Proactive issue resolution
- ✅ Learn better coding practices
- ✅ Maintain clean commit history
- ✅ Save time on manual fixes

---

### 5. Git Hook Enhancements

#### Problem
Current hook is simple prepare-commit-msg. More advanced hooks could provide better workflow integration.

#### Proposed Solutions

**A. Pre-commit hook with auto-fix:**

```bash
#!/bin/sh
# .git/hooks/pre-commit

# Run linting and auto-fix before commit
commitgen lint --fix

# Validate commit scope
commitgen validate-scope
```

**B. Commit-msg hook with validation:**

```bash
#!/bin/sh
# .git/hooks/commit-msg

# Validate message format
commitgen validate-msg --file "$1"

# Suggest improvements
if [ $? -ne 0 ]; then
    commitgen suggest-improvements --file "$1"
fi
```


**C. Post-commit analysis:**

```bash
#!/bin/sh
# .git/hooks/post-commit

# Analyze commit quality
commitgen analyze-commit HEAD

# Update team style learning
commitgen learn-style --update
```

**Installation:**

```bash
# Install all recommended hooks
commitgen hooks install --all

# Install specific hooks
commitgen hooks install --pre-commit --commit-msg

# Uninstall hooks
commitgen hooks uninstall
```

---

### 6. Workspace/Monorepo Support

#### Problem
In monorepos, changes might span multiple packages/services. CommitGen should understand this context.

#### Proposed Solution

**Detect and scope commits by workspace:**

```go
// internal/workspace/detector.go
package workspace

type Workspace struct {
    Type     string // "npm", "go-modules", "cargo", "maven"
    Root     string
    Packages []Package
}

type Package struct {
    Name string
    Path string
}

func DetectWorkspace(repoRoot string) (*Workspace, error) {
    // Check for package.json with workspaces
    if exists(filepath.Join(repoRoot, "package.json")) {
        return detectNPMWorkspace(repoRoot)
    }
    
    // Check for go.work
    if exists(filepath.Join(repoRoot, "go.work")) {
        return detectGoWorkspace(repoRoot)
    }
    
    // Check for Cargo.toml workspace
    if exists(filepath.Join(repoRoot, "Cargo.toml")) {
        return detectCargoWorkspace(repoRoot)
    }
    
    return nil, nil
}

func (w *Workspace) AffectedPackages(changes []string) []Package {
    affected := []Package{}
    for _, change := range changes {
        for _, pkg := range w.Packages {
            if strings.HasPrefix(change, pkg.Path) {
                affected = append(affected, pkg)
            }
        }
    }
    return affected
}
```


**Smart commit scoping:**

```bash
# Monorepo with changes in multiple packages
$ git status
  packages/auth/src/login.ts
  packages/api/src/routes.ts
  packages/shared/utils.ts

$ commitgen
# Detects workspace structure
# Generates scope-aware message:
feat(auth,api): implement OAuth2 login flow

- Add OAuth2 provider in auth package
- Create login endpoints in api package  
- Add shared utility functions

Affects: @myapp/auth, @myapp/api, @myapp/shared
```

**Configuration:**

```json
{
  "workspace": {
    "enabled": true,
    "group_by_package": true,
    "suggest_split_if_multi_package": true,
    "scope_format": "{{packages}}" // or "{{primary_package}}"
  }
}
```

---

### 7. CI/CD Integration Features

#### Problem
CommitGen is great for local development but doesn't integrate with CI/CD pipelines.

#### Proposed Solutions

**A. Commit message validation in CI:**

```yaml
# .github/workflows/validate-commits.yml
name: Validate Commits

on: [pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
        with:
          fetch-depth: 0
      
      - name: Install CommitGen
        run: |
          curl -sL https://github.com/hoanghonghuy/commitgen/releases/latest/download/commitgen-linux-amd64 -o commitgen
          chmod +x commitgen
      
      - name: Validate commit messages
        run: |
          ./commitgen ci validate-commits \
            --from ${{ github.event.pull_request.base.sha }} \
            --to ${{ github.event.pull_request.head.sha }}
```


**B. Auto-generate release notes:**

```bash
# Generate release notes from commits
commitgen release-notes \
  --from v1.0.0 \
  --to v1.1.0 \
  --output CHANGELOG.md \
  --format markdown

# Output:
# ## v1.1.0 (2024-01-15)
#
# ### Features
# - **auth**: Implement OAuth2 authorization ([#123])
# - **api**: Add user profile endpoint ([#124])
#
# ### Bug Fixes  
# - **db**: Resolve connection pool leak ([#125])
#
# ### Breaking Changes
# - **api**: Change user ID format to UUID
```

**C. PR description generation:**

```bash
# Generate PR description from commits
commitgen pr-description \
  --from main \
  --to feature-branch \
  --template .github/PR_TEMPLATE.md

# Output populates template with:
# - Summary of changes
# - List of commits
# - Affected files
# - Testing notes
```

---

### 8. Advanced AI Features

#### A. Multi-Turn Refinement

```bash
commitgen suggest

> Generated: "feat: update user authentication"

You: "Make it more specific about JWT"

> Regenerated: "feat(auth): implement JWT token refresh mechanism"

You: "Add that it fixes the session timeout issue"

> Final: "feat(auth): implement JWT token refresh to fix session timeout

Implements automatic token refresh when JWT expires, preventing 
unexpected logouts during active sessions."
```


#### B. Context-Aware Suggestions

```go
// internal/context/analyzer.go
package context

type CommitContext struct {
    RelatedIssues   []string // Detected from branch name, code comments
    RelatedPRs      []string
    Dependencies    []string // Changed dependencies
    Breaking        bool     // Detected breaking changes
    Migration       bool     // Database migration detected
}

func AnalyzeContext(changes []Change, branch string) CommitContext {
    ctx := CommitContext{}
    
    // Detect issue references from branch
    // e.g., "feature/JIRA-123-add-auth" -> JIRA-123
    if issue := extractIssueFromBranch(branch); issue != "" {
        ctx.RelatedIssues = append(ctx.RelatedIssues, issue)
    }
    
    // Detect dependency changes
    for _, change := range changes {
        if isDependencyFile(change.Path) {
            ctx.Dependencies = parseDependencyChanges(change.Diff)
        }
    }
    
    // Detect breaking changes
    ctx.Breaking = detectBreakingChanges(changes)
    
    // Detect migrations
    ctx.Migration = detectMigrations(changes)
    
    return ctx
}
```

**Enhanced commit message:**

```
feat(auth): implement OAuth2 authorization [JIRA-1234]

- Add OAuth2 provider with Google and GitHub support
- Implement token refresh mechanism
- Add middleware for protected routes

Dependencies:
  + oauth2-client@3.2.1
  + jwt-decoder@2.1.0

BREAKING CHANGE: Old session tokens are no longer valid.
Users will need to re-authenticate.

Migration: Run `db migrate up` to add oauth_tokens table

Closes: JIRA-1234
Related: PR #456
```

---


### 9. Plugin System

#### Problem
Users have custom needs that core tool can't address (company-specific formats, integrations, workflows).

#### Proposed Solution

**Plugin architecture:**

```go
// internal/plugin/plugin.go
package plugin

type Plugin interface {
    Name() string
    Version() string
    Init(config map[string]interface{}) error
    
    // Hooks
    PreGenerate(ctx *GenerateContext) error
    PostGenerate(ctx *GenerateContext, message string) (string, error)
    PreCommit(ctx *CommitContext) error
    PostCommit(ctx *CommitContext) error
}

type GenerateContext struct {
    Changes      []Change
    Branch       string
    RecentCommits []string
    Config       Config
}

type CommitContext struct {
    Message    string
    CommitHash string
}
```

**Plugin configuration:**

```json
{
  "plugins": {
    "jira-integration": {
      "enabled": true,
      "path": "~/.commitgen/plugins/jira.so",
      "config": {
        "url": "https://mycompany.atlassian.net",
        "project": "PROJ"
      }
    },
    "slack-notify": {
      "enabled": true,
      "path": "~/.commitgen/plugins/slack.so",
      "config": {
        "webhook_url": "https://hooks.slack.com/..."
      }
    }
  }
}
```

**Example plugin: Jira Integration**

```go
// plugins/jira/jira.go
package main

import "github.com/hoanghonghuy/commitgen/internal/plugin"

type JiraPlugin struct {
    url     string
    project string
}

func (p *JiraPlugin) PostGenerate(ctx *plugin.GenerateContext, message string) (string, error) {
    // Extract issue from branch
    issue := extractIssue(ctx.Branch)
    if issue == "" {
        return message, nil
    }
    
    // Fetch issue details from Jira
    details, err := p.fetchIssue(issue)
    if err != nil {
        return message, err
    }
    
    // Enhance message with Jira details
    enhanced := message + fmt.Sprintf("\n\nJira: %s\nType: %s\nPriority: %s", 
        details.Key, details.Type, details.Priority)
    
    return enhanced, nil
}

// Export plugin
var Plugin plugin.Plugin = &JiraPlugin{}
```


**Plugin marketplace:**

```bash
# List available plugins
commitgen plugins list

# Install plugin
commitgen plugins install jira-integration

# Enable plugin
commitgen plugins enable jira-integration

# Configure plugin
commitgen plugins configure jira-integration
```

---

## 📊 Implementation Priority Matrix

| Feature | Impact | Effort | Priority | Timeline |
|---------|--------|--------|----------|----------|
| **i18n & Localization** | High | Medium | **P1** | 2-3 weeks |
| **Git Commit Templates** | Medium | Low | **P1** | 1 week |
| **Team Style Learning** | High | Medium | **P2** | 2 weeks |
| **Commit Validation** | High | Medium | **P2** | 2 weeks |
| **Interactive Fix Suggestions** | Very High | High | **P3** | 3-4 weeks |
| **Workspace/Monorepo Support** | Medium | Medium | **P3** | 2 weeks |
| **CI/CD Integration** | Medium | Low | **P2** | 1 week |
| **Advanced AI Features** | High | High | **P4** | 4+ weeks |
| **Plugin System** | Medium | Very High | **P5** | 6+ weeks |

### Recommended Implementation Order

**Phase 1: Quick Wins (Weeks 1-2)**
1. ✅ Git Commit Templates Integration
2. ✅ CI/CD Validation Commands

**Phase 2: Core UX (Weeks 3-5)**
3. ✅ i18n & Localization
4. ✅ Commit Message Validation

**Phase 3: Intelligence (Weeks 6-8)**
5. ✅ Team Style Learning
6. ✅ Workspace/Monorepo Support

**Phase 4: Advanced Features (Weeks 9-12)**
7. ✅ Interactive Fix Suggestions
8. ✅ Context-Aware Enhancements

**Phase 5: Extensibility (Future)**
9. ✅ Plugin System
10. ✅ Marketplace & Community Plugins

---

## 🎯 Success Metrics

### User Adoption
- [ ] 50% reduction in manual commit message edits
- [ ] 80% of users enable at least one new feature
- [ ] Positive feedback on localized UI (target: 4.5/5 rating)

### Code Quality
- [ ] 30% reduction in commit message validation failures
- [ ] 25% improvement in commit message consistency
- [ ] 90% of suggested fixes are accepted

### Community
- [ ] 5+ community-contributed language translations
- [ ] 10+ community-developed plugins
- [ ] Active usage in 100+ organizations

---

## 📝 Contributing

Want to help implement these features? Check out our [CONTRIBUTING.md](../CONTRIBUTING.md) guide and pick a feature from the priority matrix above!

For questions or discussions about these improvements, open an issue with the `enhancement` label.

