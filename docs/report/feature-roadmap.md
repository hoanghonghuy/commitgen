# Feature Roadmap — commitgen

> Checklist các cải thiện & tính năng mới. Tick `[x]` khi hoàn thành.
> Nguyên tắc: giữ build/test xanh sau mỗi nhóm, theo đúng pattern hiện có.
> **Trạng thái: 22/22 hoàn thành. `go vet` sạch, `go test ./...` xanh, coverage tổng 77.6%.**

## Nhóm A — Hạ tầng & quick wins
- [x] #19 CI GitHub Actions: chạy `go vet` + `go test -race -cover` (`.github/workflows/ci.yml`)
- [x] #20 Cấu hình `golangci-lint` (`.golangci.yml`) + lint job trong CI
- [x] #21 Flag `--version` (nhúng version/commit/date qua ldflags + goreleaser)
- [x] #15 Lệnh `config show` / `config path` (in cấu hình, ẩn key; in đường dẫn)
- [x] #8 Timeout cấu hình được (`--timeout` + `timeout_seconds`), bỏ hardcode 120s
- [x] #7 Validate/clamp giá trị âm cho `recent-n` / `max-files` / `temp`

## Nhóm B — Tính năng lõi
- [x] #2 Chế độ non-interactive (`--print`): in message ra stdout, không mở TUI
- [x] #3 `--dry-run` (xem trước, không commit/không ghi hook)
- [x] #12 Lệnh `ping` / `models`: kiểm tra kết nối + liệt kê model (ollama/openai)
- [x] #11 Retry + backoff thống nhất trong `httpx` cho mọi provider
- [x] #10 Sentinel errors (`ErrNoStagedChanges`, `ErrMissingAPIKey`...) thay cho so khớp chuỗi

## Nhóm C — Cấu hình & DX
- [x] #16 Per-repo config: `.commitgen.json` trong repo ghi đè global (`LoadResolved`/`Merge`)
- [x] #17 Path-glob đầy đủ cho `ignored_files` (full-path + dir-prefix `dir/`, `dir/**`)
- [x] #18 `prompt_template_file`: trỏ tới file template

## Nhóm D — Tính năng nâng cao TUI/Provider
- [x] #1 Hook chạy được trên Windows (script `--print` non-interactive khi Windows/`--print`)
- [x] #4 `--amend`: sửa commit gần nhất (`gitx.CommitAmend`)
- [x] #5 `--count`: sinh nhiều phương án + màn hình chọn (`stateChoose`)
- [x] #6 Regenerate có hướng dẫn: nhập gợi ý (`stateRegenHint`)
- [x] #13 Streaming hiển thị token dần trên TUI (OpenAI SSE + Ollama NDJSON, fallback an toàn)
- [x] #14 Provider preset: gợi ý base-URL OpenRouter/Mistral/Azure trong form config
- [x] #22 `summarizeGo` dùng AST (`go/parser`), fallback heuristic khi source lỗi cú pháp

---

## Nhật ký triển khai

### Thay đổi chính theo file
- `cmd/commitgen/main.go`: flags `--timeout`, `--print`, `--dry-run`, `--amend`, `--count`, `--version`; `resolveCommand`/`printVersion`; load `prompt_template_file`; dùng `config.LoadResolved`.
- `internal/config`: thêm `Timeout`, `PromptTemplateFile`; `Merge`, `LoadResolved`, `findRepoLocalConfig` (giới hạn tìm tới home).
- `internal/app/commands.go` (mới): `runPing`, `runModels`, `runSuggestNonInteractive`, `generateCommitMessage` (dùng chung), `conventionalReminder`, helpers liệt kê model.
- `internal/app/errors.go` (mới): sentinel errors + `clampTemperature`/`clampNonNegative`.
- `internal/app/run.go`: dispatch `ping`/`models`/`config show|path`; nhánh non-interactive; sentinel errors; `shouldIgnore` path-glob; `resolveConfigPath`/`showConfig`/`maskSecret`.
- `internal/app/tui.go`: streaming (`streamEvent`, `startStreamCmd`/`waitStreamCmd`, view token dần), amend, count (candidates + chooser), guided regenerate.
- `internal/app/hook.go`: hook script đa nền tảng (interactive `/dev/tty` vs non-interactive `--print`).
- `internal/{openai,ollama}/client.go`: `GenerateStream` (SSE / NDJSON).
- `internal/ai/provider.go`: interface tùy chọn `StreamProvider`.
- `internal/gitx/git.go`: `CommitAmend`.
- `internal/vscodeprompt/summarize.go`: `summarizeGoAST` + fallback heuristic.
- `internal/httpx/client.go`: retry + backoff cho transient (429/5xx/network).
- `.github/workflows/ci.yml`, `.golangci.yml`, `.goreleaser.yaml` (ldflags).

### Test bổ sung
Thêm test cho: config (Merge/LoadResolved/path-glob), httpx retry, các AI client (gồm streaming), gitx (CommitAmend), app (ping/models/non-interactive/config show-path/streaming/candidates/guided regenerate), vscodeprompt (AST + fallback). Coverage tổng từ ~3% (đầu dự án) → **77.6%**.

### Phần cần lưu ý
- Streaming được hỗ trợ cho tất cả provider (OpenAI SSE, Ollama NDJSON, Anthropic/Gemini SSE).
- Validation chỉ bật khi có `.commitgen-rules.json` hoặc `rules_file` trong config (không ép mặc định).
- Git hook luôn chạy headless (`--print`), không mở TUI alternate screen.
- `main()` và `runConfigInteractive()` vẫn không unit-test được (cần TTY); đã refactor tách logic thuần để test phần còn lại.
