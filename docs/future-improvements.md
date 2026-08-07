# Future Improvements for CommitGen

This document outlines potential improvements and new features for CommitGen that would enhance user experience and functionality.

---

## Verified backlog (audit 2026-07-16)

Findings from post-`provider-config-registry` full flow audit.

### Fixed (2026-07-16)

| ID | Area | Fix |
|----|------|-----|
| VB-01 | `models` | `openrouter` / `compatible` / `ollama-cloud` list models |
| VB-08 | `--hook` alone | `shouldRunSuggestHeadless` forces non-TUI |
| VB-09 | `configAction` | `ResolveConfigAction` for `-cmd=config show` |
| VB-10 | review copy | return to `reviewStateQuickDone` when quick |
| VB-11 | clipboard | `clipboardErrMsg` surfaced in TUI |
| VB-12 | scroll hint i18n | `tui.hint.scroll_*` keys |
| VB-13 | config show | `config.file_path`; ollama+key label stays local |
| VB-14 | custom ollama URL | preserve in `CompatibleBaseURL` + resolve override |
| VB-15 | overlay load | stderr warning on failure |
| VB-16 | validation print | issues appended in `ValidationFailedError` |
| VB-17 | unknown-cmd help | includes ping/models/version/pr |
| VB-18 | `SaveConfig` | removed unused field |
| VB-19 | default models | prefer `ModelSuggestions[0]` |

### Intentional / OK (no code change)

| ID | Area | Status |
|----|------|--------|
| VB-02 | review regen no hint | by design |
| VB-03 | pr context ≠ suggest recent style | by design |
| VB-04 | print/dry-run suggest-only | by design |
| VB-05 | suggest regen guidance | OK |
| VB-06 | validator suggest-only | by design |
| VB-07 | review → suggest handoff | OK |

---

## Completed Features

### Git Commit Templates Integration

Implemented in `internal/gitx/template.go` and prompt generation. CommitGen detects configured `commit.template`, repo-local `.gitmessage`, or `.git/commit_template`, then adds the template structure to the commit-message prompt.

### Team Style Learning

Implemented in `internal/analyzer/style.go` and prompt generation. CommitGen analyzes recent commit subjects to infer common Conventional Commit types, scopes, emoji placement, ticket references, and average subject length. Ticket guidance is safety-bounded: IDs are included only if already present in branch name, staged changes, or user instructions.

### 🌐 i18n & Localization

Fully implemented in `internal/i18n/`. 130 translation keys across 4 locales (en, vi, ja, zh). TUI, CLI errors, hook messages, and config form all use `Translator.T()`. Set via `--locale`, `COMMITGEN_LOCALE`, or `locale` in config. `--locale auto` detects from `$LANG`/`$LC_ALL`/`$LC_MESSAGES`.

### Commit Message Validation

Fully implemented in `internal/validator/`. 5 rules: `SubjectLengthRule`, `BodyLineLengthRule`, `RequiredFootersRule`, `ProhibitedWordsRule`, `RequiredPatternsRule`. Auto-fix for subject-length and prohibited-words. TUI shows `stateValidationFailed` with 4 actions (Auto-fix, Edit, Ignore warnings, Cancel). `--print`/hook path rejects error-level violations via `ValidationFailedError`. Opt-in via `.commitgen-rules.json` (auto-discovered) or `rules_file` config.

---

## Future Proposals (remaining)

### 1. Interactive Fix Suggestions (AI-Powered Code Review)
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
Code Review Summary

Issues Found:
  1. Missing error handling in AuthHandler (line 45)
  2. Hardcoded API endpoint should use config
  3. Function 'processData' is too complex (cyclomatic complexity: 15)

Fix Suggestions

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
Commit Analysis

Your staged changes include:
  • 3 new features
  • 2 bug fixes
  • 1 refactoring

Recommendation: Split into 6 smaller commits

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
- Proactive issue resolution
- Learn better coding practices
- Maintain clean commit history
- Save time on manual fixes

---

### 4. Git Hook Enhancements

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

### 5. Workspace/Monorepo Support

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

### 6. CI/CD Integration Features

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

### 7. Advanced AI Features

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

### 8. Plugin System

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

## Implementation Priority Matrix

| Feature | Impact | Effort | Priority | Timeline | Status |
|---------|--------|--------|----------|----------|--------|
| **i18n & Localization** | High | Medium | ~~P1~~ | ~~2-3 weeks~~ | COMPLETED |
| **Commit Validation** | High | Medium | ~~P2~~ | ~~2 weeks~~ | COMPLETED |
| **Git Commit Templates** | Medium | Low | **P1** | 1 week | Not started |
| **Team Style Learning** | High | Medium | **P2** | 2 weeks | Not started |
| **Interactive Fix Suggestions** | Very High | High | **P3** | 3-4 weeks | Partial (review only) |
| **Workspace/Monorepo Support** | Medium | Medium | **P3** | 2 weeks | Not started |
| **CI/CD Integration** | Medium | Low | **P2** | 1 week | Not started |
| **Advanced AI Features** | High | High | **P4** | 4+ weeks | Not started |
| **Plugin System** | Medium | Very High | **P5** | 6+ weeks | Not started |

### Recommended Implementation Order

**Phase 1: Quick Wins (Weeks 1-2)**
1. Git Commit Templates Integration
2. CI/CD Validation Commands

**Phase 2: Core UX (Weeks 3-5)**
3. i18n & Localization (completed)
4. Commit Message Validation (completed)

**Phase 3: Intelligence (Weeks 6-8)**
5. Team Style Learning
6. Workspace/Monorepo Support

**Phase 4: Advanced Features (Weeks 9-12)**
7. Interactive Fix Suggestions (partial: review exists, fixer missing)
8. Context-Aware Enhancements

**Phase 5: Extensibility (Future)**
9. Plugin System
10. Marketplace & Community Plugins

---

## Success Metrics

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

## Contributing

Want to help implement these features? Check out our [CONTRIBUTING.md](../CONTRIBUTING.md) guide and pick a feature from the priority matrix above!

For questions or discussions about these improvements, open an issue with the `enhancement` label.
