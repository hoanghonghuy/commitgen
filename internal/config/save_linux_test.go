package config

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSave_WriteFailurePreservesExisting(t *testing.T) {
	for _, kind := range []string{"existing", "new", "symlink"} {
		t.Run(kind, func(t *testing.T) { testSaveWriteFailure(t, kind) })
	}
}

func testSaveWriteFailure(t *testing.T, kind string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	before := []byte(`{"provider":"openai","model":"original","api_keys":{"openai":"FAKE_FIXTURE_KEY"}}`)
	if kind != "new" {
		if err := os.WriteFile(path, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if kind == "symlink" {
		link := filepath.Join(dir, "link.json")
		if err := os.Symlink("config.json", link); err != nil {
			t.Fatal(err)
		}
		path = link
	}
	entriesBefore, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestSaveWriteFailureProcess$")
	cmd.Env = append(os.Environ(), "COMMITGEN_TEST_SAVE_PATH="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failure fixture did not reach expected write error: %v %s", err, out)
	}
	after, err := os.ReadFile(path)
	if kind == "new" {
		if !os.IsNotExist(err) {
			t.Errorf("failed save left a partial new config: content=%s err=%v", after, err)
		}
	} else if err != nil || !bytes.Equal(before, after) {
		t.Errorf("failed save destroyed old config: before=%s after=%s err=%v", before, after, err)
	}
	if kind == "symlink" {
		link, err := os.Readlink(path)
		if err != nil || link != "config.json" {
			t.Errorf("failed save replaced symlink: target=%q err=%v", link, err)
		}
	}
	entriesAfter, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entriesBefore) != len(entriesAfter) {
		t.Fatalf("failed save leaked files: before=%v after=%v", entriesBefore, entriesAfter)
	}
	for i := range entriesBefore {
		if entriesBefore[i].Name() != entriesAfter[i].Name() {
			t.Errorf("failed save changed directory entries: before=%s after=%s", entriesBefore[i].Name(), entriesAfter[i].Name())
		}
	}
}

func TestSaveWriteFailureProcess(t *testing.T) {
	path := os.Getenv("COMMITGEN_TEST_SAVE_PATH")
	if path == "" {
		return
	}
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	original := limit
	limit.Cur = 128
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Go writes subprocess coverage after this test returns.
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &original); err != nil {
			t.Errorf("restore file size limit: %v", err)
		}
	}()
	signal.Ignore(syscall.SIGXFSZ)
	err := Save(FileConfig{Provider: "openai", Model: "new", PromptTemplate: strings.Repeat("X", 512)}, path)
	if !errors.Is(err, syscall.EFBIG) {
		t.Fatalf("expected controlled EFBIG write failure, got %v", err)
	}
}
