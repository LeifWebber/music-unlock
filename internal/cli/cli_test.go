package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/LeifWebber/music-unlock/internal/discovery"
	"github.com/LeifWebber/music-unlock/internal/qmc"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../qmc/testdata/mflac_map_raw.bin")
	if err != nil {
		t.Fatal(err)
	}
	s, err := os.ReadFile("../qmc/testdata/mflac_map_suffix.bin")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, s...)
}

func put(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}

func runCLI(t *testing.T, want int, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	got := runWithDiscovery(context.Background(), args, &stdout, &stderr, "test", nil, func() ([]discovery.Source, error) { return nil, nil }, func() (string, error) {
		t.Fatal("unexpected system output directory lookup")
		return "", nil
	})
	if got != want {
		t.Fatalf("exit=%d want=%d\n%s\n%s", got, want, &stdout, &stderr)
	}
	return stdout.String() + stderr.String()
}

func TestRecursivePreservesInputsAndSkips(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "输入")
	out := filepath.Join(in, "输出")
	a := filepath.Join(in, "歌手 A", "测试.MFLAC")
	b := filepath.Join(in, "歌手 B", "测试.mflac")
	put(t, a, sample(t))
	put(t, b, sample(t))
	put(t, filepath.Join(in, "lyrics.lrc"), []byte("lyrics"))
	put(t, filepath.Join(out, "should-not-scan.mflac"), []byte("bad"))
	runCLI(t, 0, in, out)
	for _, path := range []string{a, b} {
		raw, _ := os.ReadFile(path)
		if !bytes.Equal(raw, sample(t)) {
			t.Fatal("input changed")
		}
	}
	for _, name := range []string{"歌手 A", "歌手 B"} {
		actual, err := os.ReadFile(filepath.Join(out, name, "测试.flac"))
		if err != nil {
			t.Fatal(err)
		}
		want, _ := os.ReadFile("../qmc/testdata/mflac_map_target.bin")
		if !bytes.Equal(actual, want) {
			t.Fatal("output mismatch")
		}
	}
	if msg := runCLI(t, 0, in, out); !strings.Contains(msg, "跳过 2") {
		t.Fatal(msg)
	}
	if _, err := os.Stat(filepath.Join(out, "lyrics.lrc")); !os.IsNotExist(err) {
		t.Fatal("copied unrelated file")
	}
}

func TestDryRunAndErrors(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "single.mflac")
	out := filepath.Join(dir, "out")
	put(t, in, sample(t))
	runCLI(t, 0, in, out, "--dry-run")
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote output")
	}
	runCLI(t, 2, in)
	runCLI(t, 2, in, out, "--unknown")
	runCLI(t, 2, in, out, "--offline", "--qqmusic")
	runCLI(t, 2, in, in)
	runCLI(t, 2, dir, dir)
	runCLI(t, 1, filepath.Join(dir), out, "--key-file", writeWrongKey(t, dir))
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("bad key wrote output")
	}
	put(t, in, []byte("corrupt"))
	runCLI(t, 1, in, out)
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("bad input wrote output")
	}
	if !strings.Contains(runCLI(t, 0, "--version"), "test") {
		t.Fatal("missing version")
	}
	runCLI(t, 0, "--help")
}

func writeWrongKey(t *testing.T, dir string) string {
	path := filepath.Join(dir, "wrong.json")
	b, _ := json.Marshal(map[string]string{"single.mflac": "wrong"})
	put(t, path, b)
	return path
}

func TestCollisionAndPartialFailure(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in")
	out := filepath.Join(dir, "out")
	put(t, filepath.Join(in, "same.mflac"), sample(t))
	put(t, filepath.Join(in, "same.mflac0"), sample(t))
	put(t, filepath.Join(in, "broken.mgg"), []byte("broken"))
	msg := runCLI(t, 1, in, out)
	if !strings.Contains(msg, "成功 1") || !strings.Contains(msg, "失败 2") {
		t.Fatal(msg)
	}
	files, _ := os.ReadDir(out)
	if len(files) != 1 || files[0].Name() != "same.flac" {
		t.Fatal("partial output leaked")
	}
}

func TestSymlinkEscapeAndCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires Windows privileges")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in")
	out := filepath.Join(dir, "out")
	escape := filepath.Join(dir, "escape")
	put(t, filepath.Join(in, "nested", "song.mflac"), sample(t))
	os.MkdirAll(out, 0755)
	os.MkdirAll(escape, 0755)
	if err := os.Symlink(escape, filepath.Join(out, "nested")); err != nil {
		t.Fatal(err)
	}
	runCLI(t, 1, in, out)
	items, _ := os.ReadDir(escape)
	if len(items) != 0 {
		t.Fatal("escaped output root")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	if code := Run(ctx, []string{in, out}, &output, &output, "test"); code != 130 {
		t.Fatalf("cancellation exit=%d", code)
	}
}

func TestExclusiveFallback(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	put(t, filepath.Join(dir, "temp"), []byte("audio"))
	_, status, err := copyExclusive(context.Background(), root, "temp", "song.flac", "song.flac")
	if err != nil || status != "成功" {
		t.Fatal(status, err)
	}
	put(t, filepath.Join(dir, "temp"), []byte("different"))
	_, status, err = copyExclusive(context.Background(), root, "temp", "song.flac", "song.flac")
	if err != nil || status != "跳过" {
		t.Fatal(status, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "song.flac"))
	if string(b) != "audio" {
		t.Fatal("overwrote existing output")
	}
}

func TestAutomaticProviderAndOffline(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "song.mflac")
	raw, err := os.ReadFile("../qmc/testdata/mflac_map_raw.bin")
	if err != nil {
		t.Fatal(err)
	}
	footer := make([]byte, 192)
	footer[176] = 192
	footer[180] = 1
	copy(footer[184:], "musicex\x00")
	put(t, input, append(raw, footer...))
	key, err := os.ReadFile("../qmc/testdata/mflac_map_key_raw.bin")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		args        []string
		want, calls int
	}{
		{"automatic", nil, 0, 1},
		{"offline", []string{"--offline"}, 1, 0},
		{"dry run", []string{"--dry-run"}, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			var output bytes.Buffer
			provider := func(context.Context, *qmc.MissingKeyError) (string, error) { count++; return string(key), nil }
			args := append([]string{input, filepath.Join(dir, tc.name)}, tc.args...)
			code := run(context.Background(), args, &output, &output, "test", provider)
			if code != tc.want || count != tc.calls {
				t.Fatalf("code=%d calls=%d: %s", code, count, &output)
			}
		})
	}
}
