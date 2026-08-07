# Verified Backlog (Audit 2026-07-16)

> **Status**: All items resolved as of 2026-07-16
> 
> Findings from post-`provider-config-registry` full flow audit.

## Fixed (2026-07-16)

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

## Intentional / OK (no code change)

| ID | Area | Status |
|----|------|--------|
| VB-02 | review regen no hint | by design |
| VB-03 | pr context ≠ suggest recent style | by design |
| VB-04 | print/dry-run suggest-only | by design |
| VB-05 | suggest regen guidance | OK |
| VB-06 | validator suggest-only | by design |
| VB-07 | review → suggest handoff | OK |
