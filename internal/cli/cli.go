package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/LeifWebber/music-unlock/internal/discovery"
	"github.com/LeifWebber/music-unlock/internal/ncm"
	"github.com/LeifWebber/music-unlock/internal/qmc"
	"github.com/LeifWebber/music-unlock/internal/qqmusic"
)

const usage = `用法: unmus [输出目录] [选项]
      unmus <输入文件或文件夹> <输出目录> [选项]

不传路径：自动发现音乐下载目录，输出到系统下载文件夹下的“已解锁音乐/平台”。
一个路径：自动发现来源，输出到指定目录的对应平台子目录。
两个路径：显式指定来源和输出目录。

递归转换 QQ 音乐 QMC/MFLAC/MGG 和网易云 NCM，保留目录结构和源文件。
自动识别加密方式，必要时使用本机 QQ 音乐登录态获取歌曲密钥。
默认跳过已有输出；不复制普通音频、歌词，不跟随扫描中的符号链接。

选项:
  --offline        仅离线转换，不提取登录凭据、不联网
  --dry-run        离线检查并显示目标路径，不写入文件
  --key-file PATH  手工导入 JSON 歌曲密钥映射（高级用法）
  --version        显示版本
  -h, --help       显示帮助

退出码: 0 成功/已有输出被跳过；1 转换失败或没有支持的文件；2 参数错误；130 中断。
`

// Run is separate from main so filesystem behavior and exit codes are testable.
func Run(ctx context.Context, args []string, out, errOut io.Writer, version string) int {
	return run(ctx, args, out, errOut, version, automaticProvider(errOut))
}

func automaticProvider(log io.Writer) keyProvider {
	var client *qqmusic.Client
	var loadErr error
	attempted := false
	return func(ctx context.Context, missing *qmc.MissingKeyError) (string, error) {
		if !attempted {
			attempted = true
			fmt.Fprintln(log, "正在从本机 QQ 音乐获取歌曲密钥…")
			credentials, err := qqmusic.LoadSession(ctx)
			loadErr = err
			if err == nil {
				client = qqmusic.New(credentials)
			}
		}
		if loadErr != nil {
			return "", loadErr
		}
		return client.Key(ctx, missing.MediaName, missing.SongMID)
	}
}

func run(ctx context.Context, args []string, out, errOut io.Writer, version string, provider keyProvider) int {
	return runWithDiscovery(ctx, args, out, errOut, version, provider, discovery.Downloads, discovery.DefaultOutput)
}

func runWithDiscovery(ctx context.Context, args []string, out, errOut io.Writer, version string, provider keyProvider, discover discoverSources, defaultOutput func() (string, error)) int {
	flags := flag.NewFlagSet("unmus", flag.ContinueOnError)
	flags.SetOutput(errOut)
	keyFile := flags.String("key-file", "", "JSON ekey 文件")
	dryRun := flags.Bool("dry-run", false, "仅检查")
	legacyOnline := flags.Bool("qqmusic", false, "兼容旧版；现在默认自动获取密钥")
	offline := flags.Bool("offline", false, "禁止读取登录态和联网")
	showVersion := flags.Bool("version", false, "版本")
	flags.Usage = func() { fmt.Fprint(errOut, usage) }
	// Accept options on either side of positional arguments. -- ends
	// option parsing, allowing filenames that begin with a dash.
	options, positional, err := splitArgs(args)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	if err := flags.Parse(options); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(out, "unmus (music-unlock) "+version)
		return 0
	}
	if len(positional) > 2 {
		flags.Usage()
		return 2
	}
	if *legacyOnline && *offline {
		fmt.Fprintln(errOut, "--qqmusic 与 --offline 不能同时使用")
		return 2
	}
	if *offline || *dryRun {
		provider = nil
	}
	jobs, err := plan(positional, discover, defaultOutput)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	if len(positional) < 2 {
		for _, j := range jobs {
			fmt.Fprintf(out, "扫描 %s: %s\n输出: %s\n", j.label, j.source, j.output)
		}
	}
	keys, err := loadKeys(*keyFile)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	converted, skipped, failed, found := 0, 0, 0, 0
	claimed := map[string]string{}
	for _, j := range jobs {
		root := j.source
		if !j.directory {
			root = filepath.Dir(j.source)
		}
		process := func(path string) {
			found++
			rel, _ := filepath.Rel(root, path)
			destination := j.output
			if len(positional) < 2 {
				platform := "QQ音乐"
				if strings.EqualFold(filepath.Ext(path), ".ncm") {
					platform = "网易云音乐"
				}
				destination = filepath.Join(destination, platform)
			}
			dst, status, err := convert(ctx, path, rel, destination, keys, claimed, *dryRun, provider)
			if err != nil {
				failed++
				fmt.Fprintf(errOut, "失败 %s: %v\n", path, err)
				return
			}
			if status == "跳过" {
				skipped++
			} else {
				converted++
			}
			fmt.Fprintf(out, "%s %s → %s\n", status, rel, dst)
		}
		if j.directory {
			err = filepath.WalkDir(j.source, func(path string, entry fs.DirEntry, walkErr error) error {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if walkErr != nil {
					failed++
					fmt.Fprintf(errOut, "无法扫描 %s: %v\n", path, walkErr)
					if entry != nil && entry.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if entry.IsDir() {
					for _, other := range jobs {
						if path == other.output {
							return filepath.SkipDir
						}
					}
					return nil
				}
				if entry.Type().IsRegular() && supported(filepath.Ext(path)) {
					process(path)
				}
				return nil
			})
		} else if supported(filepath.Ext(j.source)) {
			process(j.source)
		}
		if ctx.Err() != nil {
			break
		}
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		fmt.Fprintln(errOut, "已中断，源文件保留")
		return 130
	}
	if found == 0 {
		fmt.Fprintln(errOut, "没有找到支持的加密音乐文件")
		return 1
	}
	label := "成功"
	if *dryRun {
		label = "可转换"
	}
	fmt.Fprintf(out, "%s %d，跳过 %d，失败 %d\n", label, converted, skipped, failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func splitArgs(args []string) (options, positional []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			positional = append(positional, a)
			continue
		}
		options = append(options, a)
		if a == "--key-file" || a == "-key-file" {
			i++
			if i == len(args) {
				return nil, nil, errors.New("--key-file 缺少路径")
			}
			options = append(options, args[i])
		}
	}
	return
}

func loadKeys(path string) (map[string]string, error) {
	keys := map[string]string{}
	if path == "" {
		return keys, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 4<<20))
	if err := decoder.Decode(&keys); err != nil {
		return nil, fmt.Errorf("密钥文件必须是 JSON 字符串映射: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("密钥文件含多余内容")
	}
	for name, key := range keys {
		if name == "" || strings.TrimSpace(key) == "" {
			return nil, errors.New("密钥文件含空名称或空 ekey")
		}
	}
	return keys, nil
}

func resolveOutput(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Resolve the existing ancestor even when the destination doesn't yet exist.
	parent := abs
	var missing []string
	for {
		_, err := os.Lstat(parent)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		missing = append(missing, filepath.Base(parent))
		parent = filepath.Dir(parent)
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(parent)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", errors.New("输出地址必须是目录（单参数表示输出目录，指定源文件需传两个路径）")
	}
	for i := len(missing) - 1; i >= 0; i-- {
		parent = filepath.Join(parent, missing[i])
	}
	return parent, nil
}

type keyProvider func(context.Context, *qmc.MissingKeyError) (string, error)

func convert(ctx context.Context, source, rel, output string, keys map[string]string, claimed map[string]string, dry bool, provider keyProvider) (string, string, error) {
	in, err := os.Open(source)
	if err != nil {
		return "", "", err
	}
	defer in.Close()
	decoder, err := openAudio(ctx, in, source, rel, keys, provider)
	if err != nil {
		return "", "", err
	}
	dstRel := strings.TrimSuffix(rel, filepath.Ext(rel)) + decoder.Extension
	dst := filepath.Join(output, dstRel)
	// A conservative case-insensitive collision check is portable to default
	// macOS/Windows filesystems, including when dry-running on Linux.
	claim := strings.ToLower(dst)
	if previous, exists := claimed[claim]; exists {
		return dst, "", fmt.Errorf("输出名称与 %s 冲突", previous)
	}
	claimed[claim] = source
	if st, err := os.Lstat(dst); err == nil {
		if !st.Mode().IsRegular() {
			return dst, "", errors.New("输出位置已被目录或符号链接占用")
		}
		return dst, "跳过", nil
	} else if !os.IsNotExist(err) {
		return dst, "", err
	}
	if dry {
		return dst, "计划", nil
	}
	// os.Root confines writes, including symlink resolution, to the output tree.
	if err := os.MkdirAll(output, 0o755); err != nil {
		return dst, "", err
	}
	root, err := os.OpenRoot(output)
	if err != nil {
		return dst, "", err
	}
	defer root.Close()
	parent := filepath.Dir(dstRel)
	if err := root.MkdirAll(parent, 0o755); err != nil {
		return dst, "", err
	}
	return writeOutput(ctx, root, dstRel, dst, decoder)
}

func supported(ext string) bool { return strings.EqualFold(ext, ".ncm") || qmc.Supported(ext) }

type audioStream struct {
	io.Reader
	Extension string
}

func openAudio(ctx context.Context, in *os.File, source, rel string, keys map[string]string, provider keyProvider) (*audioStream, error) {
	if strings.EqualFold(filepath.Ext(source), ".ncm") {
		d, err := ncm.Open(in)
		if err != nil {
			return nil, err
		}
		return &audioStream{d, d.Extension}, nil
	}
	decoder, err := qmc.Open(in, func(media string) string {
		for _, name := range []string{filepath.ToSlash(rel), media} {
			if key := keys[name]; key != "" {
				return strings.TrimSpace(key)
			}
		}
		return ""
	})
	var missing *qmc.MissingKeyError
	if errors.As(err, &missing) && provider != nil && missing.Kind == "musicex" {
		key, keyErr := provider(ctx, missing)
		if keyErr != nil {
			return nil, keyErr
		}
		decoder, err = qmc.Open(in, func(string) string { return key })
	}
	if err != nil {
		return nil, err
	}
	return &audioStream{decoder, decoder.Extension}, nil
}
