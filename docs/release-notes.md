# unmus v0.2.0

将 QQ 音乐和网易云 NCM 本地文件转换为普通播放器可以播放的 FLAC、OGG 或 MP3。

- 独立可执行文件：macOS、Windows、Linux，分别提供 ARM64 和 x86-64 版本。
- `unmus`：自动发现音乐下载目录，输出到系统下载文件夹下的 `已解锁音乐/平台`。
- `unmus "输出目录"`：自动发现来源，统一输出到指定位置。
- `unmus "源文件或目录" "输出目录"`：转换指定文件或递归扫描目录。
- 保留源文件、相对目录和音质，已有输出默认跳过。
- NCM 离线转换并保留歌曲信息和内嵌封面；QQ 音乐按需使用本机当前账号获取歌曲密钥。

macOS / Linux 安装：

```sh
curl -fsSL https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.sh | sh
```

一次性运行：

```sh
curl -fsSL https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.sh | sh -s -- --run
```

安装器自动选择架构并检查 SHA-256，无需 Go、Python、Node.js 或 Rust。Windows 请下载对应 ZIP 并运行 `unmus.exe`。

macOS QQ 音乐 11.7.0.2 已使用真实样本验证；Windows 的客户端登录态兼容性仍待实机验证。macOS 程序暂未进行 Developer ID 签名和公证。音频头检查不替代完整解码校验。
