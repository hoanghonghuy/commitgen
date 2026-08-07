# Audit: `install-hook` — Vấn đề & Đề xuất

**Ngày:** 2026-05-30
**File liên quan:** `internal/app/hook.go`, `internal/app/run.go`

---

## Vấn đề 1: Hook mode vẫn chạy TUI (alternate screen) ✅ CHÍNH XÁC

**Vị trí:** `internal/app/run.go` dòng 100–162

**Mô tả:** Khi hook gọi `commitgen --hook <file>`, command vẫn là `"suggest"` (mặc định), nên code luôn đi vào branch `case "suggest"` và khởi tạo TUI với `tea.WithAltScreen()`.

**Hậu quả:** Alternate screen khi chạy trong git hook gây lỗi hiển thị, terminal bị rối. Đáng lẽ khi có `--hook`, cần bỏ qua TUI → chỉ sinh message → ghi file rồi exit.

**Sửa:** Trong `run.go`, kiểm tra `cfg.HookFile != ""` trước khi khởi tạo TUI. Nếu có hook file, chạy chế độ headless (không TUI, không alternate screen).

---

## Vấn đề 2: Hook script không kiểm tra exit code ✅ CHÍNH XÁC

**Vị trí:** `internal/app/hook.go` dòng 76

```sh
"%s" --hook "$COMMIT_MSG_FILE" < /dev/tty > /dev/tty
```

**Mô tả:** Nếu commitgen crash hoặc lỗi, script vẫn tiếp tục và git sẽ commit với message rỗng hoặc message cũ.

**Sửa:**
```sh
if ! "%s" --hook "$COMMIT_MSG_FILE" < /dev/tty > /dev/tty; then
    echo "commitgen failed, aborting commit"
    exit 1
fi
```

---

## Vấn đề 3: Thiếu skip cho merge / squash / amend ✅ CHÍNH XÁC

**Vị trí:** `internal/app/hook.go` dòng 65–67

Hiện tại chỉ skip khi `$COMMIT_SOURCE = "message"` (user đã truyền `-m`).

**Thiếu các trường hợp:**
- `"merge"` — đang merge commit, không nên override message
- `"squash"` — đang squash commit
- `"commit"` — amend commit (`git commit --amend`)

**Sửa:** Thêm điều kiện skip:
```sh
if [ "$COMMIT_SOURCE" = "message" ] || [ "$COMMIT_SOURCE" = "merge" ] || [ "$COMMIT_SOURCE" = "squash" ] || [ "$COMMIT_SOURCE" = "commit" ]; then
  exit 0
fi
```

---

## Vấn đề 4: Không backup hook cũ ✅ CHÍNH XÁC

**Vị trí:** `internal/app/hook.go` dòng 31–36

Nếu đã có hook `prepare-commit-msg`, code báo lỗi và thoát. Người dùng có hook cũ quan trọng sẽ bị mất nếu không biết backup.

**Sửa:** Tự động rename hook cũ thành `prepare-commit-msg.bak` trước khi ghi file mới, hoặc hỏi người dùng (nếu chạy interactive).

---

## Vấn đề 5: TTY redirection thừa ⚠️ CHÍNH XÁC (minor)

**Vị trí:** `internal/app/hook.go` dòng 71–76

```sh
if [ -t 0 ]; then
    exec < /dev/tty
fi
...
"%s" --hook "$COMMIT_MSG_FILE" < /dev/tty > /dev/tty
```

Block `if [ -t 0 ]` đã redirect stdin, nhưng dòng sau lại redirect lại `< /dev/tty > /dev/tty`. Block `if` không có tác dụng thực tế, có thể xóa.

---

## Vấn đề 6: Config path không truyền vào hook ✅ CHÍNH XÁC

**Mô tả:** Hook gọi `commitgen --hook <file>` nhưng không truyền `--config`. Nếu người dùng lưu config ở path custom (không phải `~/.commitgen.json`), hook sẽ không dùng được config đó.

**Sửa:** Hook script nên truyền `--config` nếu phát hiện file config tồn tại, hoặc dùng biến môi trường `COMMITGEN_CONFIG`.

---

## Vấn đề 7: Phát hiện repo chưa hỗ trợ git worktree hoàn hảo ⚠️ CHÍNH XÁC (edge case)

**Vị trí:** `internal/app/hook.go` dòng 18–21

```go
gitDir := ".git"
if _, err := os.Stat(gitDir); os.IsNotExist(err) {
    return fmt.Errorf("...")
}
```

Git worktree dùng `.git` là **file** (chứa đường dẫn đến repo chính), không phải directory. `os.Stat` vẫn hoạt động với file, nhưng nếu trong tương lai code kiểm tra `IsDir()` thì sẽ fail. Nên dùng `git rev-parse --git-dir` để phát hiện repo thay vì check `.git` trực tiếp.

---

## Tóm tắt mức độ ưu tiên

| Mức | Vấn đề | Trạng thái |
|-----|--------|------------|
| **CRITICAL** | 1. Hook mode vẫn chạy TUI với alternate screen | ✅ Đã xác thực |
| **HIGH** | 2. Hook script không check exit code | ✅ Đã xác thực |
| **HIGH** | 3. Thiếu skip cho merge/squash/amend | ✅ Đã xác thực |
| **MEDIUM** | 4. Không backup hook cũ | ✅ Đã xác thực |
| **MEDIUM** | 6. Config path không truyền vào hook | ✅ Đã xác thực |
| **LOW** | 5. TTY redirection thừa | ✅ Đã xác thực |
| **LOW** | 7. `.git` detection chưa support worktree hoàn hảo | ✅ Đã xác thực |
