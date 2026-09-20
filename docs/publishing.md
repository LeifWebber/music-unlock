# 发布

项目只通过 GitHub Releases 分发独立二进制；macOS/Linux 使用 curl 安装或一次性运行，Windows 使用 ZIP。不发布到包管理器。

## 本地验证

```sh
go vet ./...
go test -race ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
shellcheck -s sh scripts/install.sh
python3 scripts/release.py --version v0.2.0
```

`dist/` 中会生成 macOS、Windows、Linux 的 ARM64/x86-64 压缩包、`SHA256SUMS` 和 `install.sh`。每份压缩包包含程序、README、文档及授权信息；个人歌曲、密钥和客户端数据不进入构建。

本地构建用于验证。正式发布使用 GitHub Actions 同一次构建产生的压缩包和校验和，不混用本地重建文件。

## 正式发布

1. 更新 `docs/release-notes.md` 的版本、变化和已知限制。
2. 将相关修改提交、推送到 `main`，确认 Test 工作流通过。
3. 创建与推送版本标签，例如 `v0.2.0`。

`.github/workflows/release.yml` 会再次运行 macOS、Windows、Linux 的测试，通过后构建六种架构的程序，验证打包后的 Linux 程序，再创建公开 GitHub Release。任一步失败都不会进入发布步骤。

Release 附件包含六个压缩包、`SHA256SUMS` 和 `install.sh`。手动触发工作流只生成验证产物，不发布；已公开版本的标签与文件不覆盖，有修复时发布新版本。

## 发布后验证

使用公开入口检查一次性运行及真实安装：

```sh
curl -fsSL https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.sh \
  | sh -s -- --run --version

curl -fsSL https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.sh \
  | sh -s -- --bin-dir ./test-install

./test-install/unmus --version
```

再用自己的本地样本确认实际转换。下载校验和、程序版本、帮助信息和音频结果应来自同一个版本。

## 安装器选项

- 默认选取最新正式 Release；`--version v0.2.0` 固定版本。
- 默认安装到 `~/.local/bin`；`--bin-dir DIR` 指定位置。
- `--run` 下载后立即运行，后面的所有参数传给 `unmus`，结束后清理临时程序。
- 已有 `unmus` 默认不覆盖；升级时传 `--force`，目录和符号链接仍不替换。
- 不需要 sudo，不修改用户 shell 配置。目标目录未在 `PATH` 时会给出提示。
