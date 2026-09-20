package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LeifWebber/music-unlock/internal/discovery"
	"github.com/LeifWebber/music-unlock/internal/qmc"
)

func TestDefaultAndSingleOutputMixedLibraries(t *testing.T) {
	for _, mode := range []string{"zero", "one"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			qq := filepath.Join(base, "QQMusic")
			net := filepath.Join(base, "CloudMusic")
			ncmData, err := os.ReadFile("../ncm/testdata/tagged-flac.ncm")
			if err != nil {
				t.Fatal(err)
			}
			put(t, filepath.Join(qq, "歌手", "same.mflac"), sample(t))
			put(t, filepath.Join(net, "歌手", "same.NCM"), ncmData)
			finder := func() ([]discovery.Source, error) {
				return []discovery.Source{{Path: qq, Platform: "QQ音乐"}, {Path: net, Platform: "网易云音乐"}}, nil
			}
			out := filepath.Join(base, "system Downloads", "已解锁音乐")
			defaultCalls := 0
			defaultOutput := func() (string, error) {
				defaultCalls++
				if mode == "one" {
					t.Fatal("explicit output must not look up the system Downloads folder")
				}
				return out, nil
			}
			var args []string
			if mode == "one" {
				out = filepath.Join(qq, "nested output")
				args = []string{out}
			}
			// Automatic mode must exclude the entire output tree across all sources.
			put(t, filepath.Join(out, "old", "bad.ncm"), []byte("not a source"))
			providerCalls := 0
			provider := func(context.Context, *qmc.MissingKeyError) (string, error) { providerCalls++; return "", nil }
			var log bytes.Buffer
			if code := runWithDiscovery(context.Background(), args, &log, &log, "test", provider, finder, defaultOutput); code != 0 {
				t.Fatalf("%d: %s", code, &log)
			}
			if providerCalls != 0 || !strings.Contains(log.String(), "成功 2") {
				t.Fatal(log.String())
			}
			if mode == "zero" && defaultCalls != 1 {
				t.Fatalf("default directory looked up %d times for two sources", defaultCalls)
			}
			if _, err := os.Stat(filepath.Join(base, "已解锁音乐")); !os.IsNotExist(err) {
				t.Fatal("created output beside source instead of in Downloads")
			}
			for _, platform := range []string{"QQ音乐", "网易云音乐"} {
				if _, err := os.Stat(filepath.Join(out, platform, "歌手", "same.flac")); err != nil {
					t.Fatal(err)
				}
			}
			log.Reset()
			if code := runWithDiscovery(context.Background(), args, &log, &log, "test", provider, finder, defaultOutput); code != 0 || !strings.Contains(log.String(), "跳过 2") {
				t.Fatalf("%d: %s", code, &log)
			}
			actual, _ := os.ReadFile(filepath.Join(net, "歌手", "same.NCM"))
			if !bytes.Equal(actual, ncmData) {
				t.Fatal("changed NCM source")
			}
		})
	}
}

func TestDefaultDryRunMissingAndExplicitSource(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "QQMusic")
	put(t, filepath.Join(src, "song.mflac"), sample(t))
	calls := 0
	finder := func() ([]discovery.Source, error) {
		calls++
		return []discovery.Source{{Path: src, Platform: "QQ音乐"}}, nil
	}
	var log bytes.Buffer
	output := filepath.Join(base, "Downloads", "已解锁音乐")
	defaultOutput := func() (string, error) { return output, nil }
	if code := runWithDiscovery(context.Background(), []string{"--dry-run"}, &log, &log, "test", nil, finder, defaultOutput); code != 0 {
		t.Fatalf("%d: %s", code, &log)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("dry run wrote output")
	}
	calls = 0
	log.Reset()
	defaultFailure := func() (string, error) { return "", errors.New("system folder lookup failed") }
	if code := runWithDiscovery(context.Background(), []string{src, filepath.Join(base, "explicit")}, &log, &log, "test", nil, finder, defaultFailure); code != 0 || calls != 0 {
		t.Fatalf("%d calls=%d %s", code, calls, &log)
	}
	log.Reset()
	if code := runWithDiscovery(context.Background(), nil, &log, &log, "test", nil, func() ([]discovery.Source, error) { return nil, nil }, defaultOutput); code != 2 || !strings.Contains(log.String(), "未找到") {
		t.Fatalf("%d: %s", code, &log)
	}
	log.Reset()
	if code := runWithDiscovery(context.Background(), nil, &log, &log, "test", nil, finder, defaultFailure); code != 2 || !strings.Contains(log.String(), "无法定位系统下载文件夹") {
		t.Fatalf("%d: %s", code, &log)
	}
}

func TestNCMFailureDoesNotBlockOtherFiles(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "input")
	out := filepath.Join(base, "output")
	valid, err := os.ReadFile("../ncm/testdata/tagged-mp3.ncm")
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(src, "good.ncm"), valid)
	put(t, filepath.Join(src, "bad.ncm"), []byte("CTENFDAM"))
	msg := runCLI(t, 1, src, out, "--offline")
	if !strings.Contains(msg, "成功 1") || !strings.Contains(msg, "失败 1") {
		t.Fatal(msg)
	}
	files, err := os.ReadDir(out)
	if err != nil || len(files) != 1 || files[0].Name() != "good.mp3" {
		t.Fatalf("output=%v, %v", files, err)
	}
}
