# Báo cáo rà soát code — commitgen

> Ngày rà soát: 2026-06-09
> Phạm vi: toàn bộ `cmd/`, `internal/*`; đối chiếu README, `.gitignore`.
> Kết quả build/test: `go vet ./...` sạch, `go build ./...` sạch, `go test ./...` PASS.

## Checklist khắc phục

### Lỗi chức năng (ưu tiên cao)
- [x] #1 — `ignored_files` trong config bị bỏ qua hoàn toàn (`cmd/commitgen/main.go`) ✅ đã sửa
- [x] #2 — Anthropic bỏ qua tham số `temperature` (`internal/anthropic/client.go`) ✅ đã sửa
- [x] #3 — `runConfig` in sai đường dẫn file đã lưu khi `-config` rỗng (`internal/app/run.go`) ✅ đã sửa

### Độ bền / đúng đắn (ưu tiên trung bình)
- [x] #4 — OpenAI không kiểm tra `resp.StatusCode`; retry sai với lỗi auth (`internal/openai/client.go`) ✅ đã sửa
- [x] #5 — Cắt chuỗi theo byte làm vỡ ký tự UTF-8 (`internal/app/run.go`) ✅ đã sửa
- [x] #6 — `git log -n N` truyền tham số sai dạng (`internal/gitx/git.go`) ✅ đã sửa
- [x] #7 — `install-hook`/`uninstall-hook` bỏ qua `--repo`, lỗi với worktree/submodule (`internal/app/hook.go`) ✅ đã sửa
- [x] #8 — `getLogPath` in thông báo log path gây hiểu nhầm (`cmd/commitgen/main.go`) ✅ đã sửa

### Vệ sinh repo / lặt vặt (ưu tiên thấp)
- [x] #9 — `tmp_test_api/go.mod` bị commit (folder tạm còn sót) ✅ đã xóa
- [ ] #10 — Binary `commitgen`/`commitgen.exe` + `.git/.MERGE_MSG.swp` còn trong working tree (binary đã `.gitignore`; `.swp` nằm trong `.git`, không can thiệp)
- [x] #11 — Gemini dùng `omitempty` trên struct value (không tác dụng) ✅ đã sửa (đổi sang con trỏ)
- [x] #12 — Anthropic `max_tokens=1024` cố định có thể cắt cụt báo cáo review ✅ đã nâng lên 4096
- [ ] #13 — `summarizeGo` đếm ngoặc đơn giản hóa, dễ sai với closure/string literal (giữ nguyên — giới hạn đã biết, rủi ro thấp)
- [ ] #14 — Ghi chú bảo mật: tránh log URL Gemini (chứa API key) — hiện chưa log URL nên không rò rỉ; giữ làm lưu ý

---

## Chi tiết

### Lỗi chức năng thực sự

#### #1 — `ignored_files` trong config bị bỏ qua hoàn toàn
**File:** `cmd/commitgen/main.go`
Khi dựng `app.Config`, struct `cfg` **không** gán `IgnoredFiles` từ `fileCfg.IgnoredFiles`.
Trong khi đó `runConfigInteractive` vẫn cho người dùng nhập "Ignored Files" và `config.Save`
vẫn ghi vào `~/.commitgen.json`.
**Hệ quả:** danh sách bỏ qua người dùng cấu hình không bao giờ có tác dụng; chỉ còn
`defaultIgnores` hardcode trong `run.go` chạy.
**Hướng sửa:** thêm `IgnoredFiles: fileCfg.IgnoredFiles` vào phần khởi tạo `cfg`.

#### #2 — Anthropic bỏ qua `temperature`
**File:** `internal/anthropic/client.go`
Hàm `generate` nhận tham số `temperature` nhưng không bao giờ đưa vào `messageRequest`
(struct không có field `temperature`).
**Hệ quả:** cấu hình temperature không có hiệu lực với Claude, khác với openai/gemini/ollama
(đều có truyền). Thiếu nhất quán, dễ gây nhầm.
**Hướng sửa:** thêm field `Temperature float64 \`json:"temperature,omitempty"\`` vào
`messageRequest` và gán giá trị.

#### #3 — `runConfig` in sai đường dẫn lưu
**File:** `internal/app/run.go`
Khi chạy không có `-config`, `cfg.ConfigPath` rỗng. `config.Save("")` tự fallback về
`~/.commitgen.json` (đúng), nhưng dòng
`fmt.Printf("Configuration saved to %s\n", cfg.ConfigPath)` in ra đường dẫn rỗng.
**Hệ quả:** người dùng không biết file thực sự lưu ở đâu.
**Hướng sửa:** tính đường dẫn thực tế (resolve về `~/.commitgen.json` khi rỗng) rồi in.

### Vấn đề độ bền / đúng đắn

#### #4 — OpenAI không kiểm tra `resp.StatusCode`
**File:** `internal/openai/client.go`
Nhánh non-streaming chỉ đọc body rồi dựa vào field JSON `error`. Nếu gateway/proxy trả HTML
(502/429/401 không phải JSON), `json.Unmarshal` thất bại → trả "decode error".
Ngoài ra logic retry kiểm tra chuỗi `"401"`/`"403"` trong lỗi, nhưng lỗi của openai client
**không chứa mã status**, nên lỗi auth vẫn bị retry 2 lần vô ích (tốn thời gian + gọi API thừa).
**Hướng sửa:** kiểm tra `resp.StatusCode` tường minh và đưa status vào thông điệp lỗi.

#### #5 — Cắt chuỗi theo byte làm vỡ ký tự UTF-8
**File:** `internal/app/run.go` (`buildPromptData`)
`ch.Diff[:2000]` và `orig[:2000]` cắt theo byte. Với nội dung tiếng Việt/Nhật (multibyte),
điểm cắt có thể rơi giữa một ký tự (rune), tạo UTF-8 không hợp lệ gửi cho LLM.
**Hướng sửa:** cắt theo rune (qua `[]rune` hoặc lùi về `utf8.RuneStart`).

#### #6 — `git log -n N` truyền tham số sai dạng
**File:** `internal/gitx/git.go`
`fmt.Sprintf("-n %d", n)` tạo **một** arg `"-n 5"` (có dấu cách bên trong) thay vì hai arg
`"-n", "5"`. Hiện chạy được do git tha thứ khoảng trắng đầu khi parse số, nhưng dễ vỡ và
không chuẩn.
**Hướng sửa:** dùng `"-n", strconv.Itoa(n)` hoặc `fmt.Sprintf("-n%d", n)`.

#### #7 — `install-hook`/`uninstall-hook` bỏ qua `--repo` và giả định CWD
**File:** `internal/app/hook.go`
Hai lệnh hardcode `gitDir := ".git"` (chỉ đúng khi đứng tại repo root), trong khi
`suggest`/`review` lại tôn trọng `-repo` và đi ngược lên tìm root → thiếu nhất quán.
Với worktree/submodule, `.git` là **file** chứ không phải thư mục → `os.Stat(".git")` không
báo lỗi nên qua được check, nhưng `MkdirAll(".git/hooks")` sẽ thất bại.
**Hướng sửa:** dùng `gitx.ResolveRepoRoot` (và `git rev-parse --git-path hooks`) thay vì
hardcode.

#### #8 — `getLogPath` gây hiểu nhầm
**File:** `cmd/commitgen/main.go`
Khi có lỗi, luôn in `Check logs at: ~/.commitgen/commitgen.log` kể cả khi `LogOutput = "stderr"`
(không hề ghi file). Tham số đặt tên `configPath` nhưng thực chất là log file path.
**Hướng sửa:** chỉ in gợi ý log file khi output có ghi file; đổi tên tham số cho đúng nghĩa.

### Vệ sinh repo / lặt vặt

#### #9 — `tmp_test_api/go.mod` bị commit
Folder tạm còn sót (module `test_qoder`, khai báo `go 1.26.3` — phiên bản Go không tồn tại).
Đã xác nhận bị track qua `git ls-files`.
**Hướng sửa:** `git rm -r tmp_test_api/`.

#### #10 — Binary và file rác trong working tree
`commitgen` / `commitgen.exe` nằm trong working tree (đã `.gitignore`, không bị track — OK),
và còn `.git/.MERGE_MSG.swp` (file swap vim, vô hại nhưng nên xóa).

#### #11 — Gemini `omitempty` trên struct value
**File:** `internal/gemini/client.go`
`GenerationConfig generationConfig \`json:"generationConfig,omitempty"\`` — `omitempty` không
có tác dụng trên struct value (không bao giờ "rỗng"), nên luôn gửi field. Vô hại nhưng dễ
hiểu nhầm ý đồ.

#### #12 — Anthropic `max_tokens=1024` cố định
**File:** `internal/anthropic/client.go`
Đủ cho commit message, nhưng báo cáo `review` (nhất là full review 4 mục) có thể bị cắt cụt.
**Hướng sửa:** tăng giá trị cho luồng review, hoặc cho cấu hình.

#### #13 — `summarizeGo` best-effort, dễ sai
**File:** `internal/vscodeprompt/summarize.go`
Comment trong code đã thừa nhận cách đếm ngoặc đơn giản hóa. Với hàm có closure lồng nhau hoặc
`}` nằm trong chuỗi/string literal, việc thu gọn có thể sai. Chấp nhận được nhưng cần biết giới hạn.

#### #14 — Ghi chú bảo mật: URL Gemini chứa API key
**File:** `internal/gemini/client.go`, `internal/logger/logger.go`
Gemini đặt API key trong query string (`?key=...`). Hiện client Gemini không log URL nên chưa
rò rỉ, nhưng cơ chế redact trong logger chỉ lọc theo key (`api_key`, `token`...) chứ không lọc
URL. Cần tránh log URL Gemini đầy đủ về sau. Cơ chế redact hiện gần như không được dùng vì
không có chỗ nào log với các key đó.

---

## Đánh giá tổng quan
Code build/test pass, kiến trúc tách lớp rõ ràng (provider interface, `gitx`, `vscodeprompt`).
Hai issue đáng sửa nhất là **#1 (ignored_files vô hiệu)** và **#2 (Anthropic bỏ qua temperature)**
vì làm cấu hình người dùng âm thầm không hoạt động.

---

## Triển khai test (cập nhật)

Đã bổ sung test toàn diện (Unit Test + Integration Test). Tất cả `go vet`, `go test ./...` đều xanh.

### Độ phủ theo package

| Package | Trước | Sau |
|---|---|---|
| `internal/ollama` | 0% | 100% |
| `internal/gemini` | 0% | 100% |
| `internal/anthropic` | 0% | 95.8% |
| `internal/httpx` | 0% | 94.7% |
| `internal/config` | 0% | 91.1% |
| `internal/openai` | 0% | 89.6% |
| `internal/vscodeprompt` | 35.9% | 90.0% |
| `internal/logger` | 52.9% | 86.8% |
| `internal/gitx` | 0% | 85.6% |
| `internal/app` | 1.0% | 71.2% |
| `cmd/commitgen` | 0% | 25.4% |
| **Tổng dự án** | **~3%** | **78.0%** |

> Lần nâng coverage thứ hai bổ sung: test nhánh home-dir của `config`, `ResolveRepoRoot` từ cwd/subdir của `gitx`, `getDefaultLogPath`/`openLogFile`/`Close` của `logger`, `renderTemplate` lỗi + `summarizeGo` cạnh của `vscodeprompt`, streaming rỗng + retry của `openai`, refactor `resolveCommand` (tách khỏi `main()`), và test `Run()` cho các nhánh không cần TTY (`dump-prompt`, `install-hook`, `uninstall-hook`, lệnh sai, các nhánh lỗi).

### Loại test đã thêm
- **Unit test (UT):** `config` (resolve/load/save), `vscodeprompt` (extract code block, summarize Go/Markdown, role mapping, language vi), `logger` (output modes, redaction, JSON), các helper TUI (`calcInnerWidth/Height`, `countLines`, `scrollHintText`, `formatReviewText`, `applyInlineStyles`), `truncateUTF8`, `newProvider`.
- **Integration test (IT):**
  - AI client (`openai`, `anthropic`, `gemini`, `ollama`): mock bằng `httptest.Server`, không cần API key thật. Kiểm tra mapping role, system prompt, temperature, xử lý lỗi HTTP, retry, SSE streaming.
  - `gitx`: tạo git repo thật trong `t.TempDir()` (init, commit, stage) và kiểm tra `StagedChanges`, `RecentCommits`, `OriginalFileAtHEAD`, `Commit`, `ResolveRepoRoot`...
  - `app`: `buildPromptData` (qua repo tạm), `InstallHook`/`UninstallHook`/`resolveHooksDir` (qua repo tạm), `dumpPrompt`, và các chuyển trạng thái của `tuiModel`/`reviewModel` (`Update`) bằng provider giả + `tea.KeyMsg`.

### Refactor nhỏ phục vụ testability
- `anthropic` và `gemini`: thêm field nội bộ `baseURL` (mặc định trỏ tới endpoint thật, **hành vi không đổi**) để test có thể inject `httptest` server. Trước đây URL bị hardcode nên không thể test mà không gọi API thật.

### Phần chưa phủ (cố ý)
- `cmd/commitgen` `main()` và `internal/app` `Run()` / `runConfigInteractive()`: mở TUI/form tương tác cần terminal thật (TTY), không phù hợp unit test. Đây là lớp orchestration mỏng; logic lõi bên dưới đã được test.

### Về TDD
TDD (viết test trước khi viết code) là quy trình áp dụng cho **code mới** về sau, không phải việc bổ sung test cho code có sẵn (đó là "retrofit test" như vừa làm). Khuyến nghị: từ giờ với tính năng/bugfix mới, viết test thất bại trước → code cho pass → refactor.
