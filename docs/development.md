# 开发

## 构建

需要 Go 1.25 或更新版本：

```sh
git clone https://github.com/LeifWebber/music-unlock.git
cd music-unlock
go mod download
go build -trimpath -o dist/unmus ./cmd/unmus
./dist/unmus ./input ./output
```

Windows 将输出文件名改为 `dist/unmus.exe`。正常使用预编译程序不需要 Go、Python、Node 或 FFmpeg。

## 验证

```sh
go test -race ./...
go vet ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
shellcheck -s sh scripts/install.sh
```

Go 测试包含上游公开向量、不同分块读取、musicex、错误密钥、损坏尾标、文件冲突、目录逃逸、默认自动模式/离线模式，以及脱敏登录记录解析。
Windows CI 另用专门启动的测试子进程验证只读内存读取，不读取真实客户端或个人账号。
测试不会访问真实 QQ 音乐账号或接口。开发者如需真实回归，应主动调用 CLI 并明确选择自己的输入文件。

## 代码结构

- `cmd/unmus`：程序入口。
- `internal/cli`：路径、批量转换、输出发布与退出码。
- `internal/discovery`：下载目录发现、系统“音乐”目录与路径去重。
- `internal/ncm`：网易云离线解密及 FLAC/ID3 标签回填。
- `internal/qmc`：解密、尾标解析和音频格式识别。
- `internal/qqmusic`：平台登录态读取与 QQ 音乐密钥接口。
- `scripts/install.sh`、`install.ps1`、`install.cmd`：校验后安装或单次运行。
- `scripts/release.py`：六种平台构建、压缩包与 SHA-256 校验和。
- `legacy/windows`：原 Python/Frida 实现，归档保留，不参与构建。

## 音频边界

在本机 macOS QQ 音乐 11.7.0.2 的两份 musicex/MFLAC 样本中，最后一帧后有相同的 15 字节非音频标记。
只有标记完全匹配、最后一帧 CRC-8/CRC-16 正确且采样终点与 STREAMINFO 一致时才移除它；检查最多读取末尾 1 MiB。
真实回归已验证 FLAC PCM MD5、总采样数、完整解码和源文件 SHA-256。另有 MGG/Vorbis 全曲解码回归。
这些个人样本只存放于被 Git 忽略的 `data/`，测试仓库里的 `tone.flac` 是本地生成的一秒正弦波。

Windows 的原生系统调用可在 Windows CI 通过合成进程验证；这不能替代真实 QQ 音乐客户端回归。
macOS 安装包暂未做 Developer ID 签名和公证。

## NCM 回归

`internal/ncm/testdata/generate.py` 使用本地生成的正弦波、Python 参考流密码和 OpenSSL AES 生成固定的 NCM 测试向量；重建需要 FFmpeg/OpenSSL，日常测试及用户运行不需要。测试覆盖 FLAC/MP3、标签与封面、缺少元数据、封面预留区、分块读取、异常长度、损坏填充与截断头。`go test ./internal/ncm -fuzz FuzzOpen -fuzztime 10s` 可执行有界模糊测试。

真实 NCM 样本已验证 96 kHz / 24-bit 双声道 FLAC、156 秒时长、标签和封面、FFmpeg 全曲解码。音频帧与独立 Python/OpenSSL 解密结果逐字节一致，解码 PCM MD5 与 FLAC STREAMINFO 一致；源文件 SHA-256 保持不变。个人样本、完整解密参考及报告不进入版本库。

Windows CI 分别在 PowerShell 5.1 和 7 中测试安装器，覆盖校验失败、参数与退出码、路径含空格/中文、PATH 去重、归档异常及 CMD 引导清理；另从公开 Release 验证 IEX 安装和一次性运行。可以在 Windows 上执行 `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_install_windows.ps1` 重现契约测试。
