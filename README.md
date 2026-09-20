# music-unlock

日常命令是 **`unmus`**，取自 **UN**lock **MUS**ic。

**把加密音乐文件，转换成普通播放器也能播放的音乐。**

支持 QQ 音乐和网易云音乐的加密文件。可自动发现下载目录，也可转换指定歌曲或整个文件夹。保留原文件、目录结构和原有音质，通常无需手动准备歌曲密钥。

对于 QQ 音乐，很可能还需要你本身有 QQ 音乐会员并且本机打开客户端程序。

截止 QQ 音乐 11.7.0.2 和网易云音乐 3.1.12 (3443) ，该工具有效。

## 安装

### macOS / Linux

一条命令安装到 `~/.local/bin`：

```sh
curl -fsSL https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.sh | sh
```

安装脚本会选择当前系统和 CPU 对应的程序，并验证 SHA-256。如果该目录尚未加入 `PATH`，脚本会提示你。
目标位置已有 `unmus` 时会停止，避免覆盖其他程序；更新本工具时可在管道后使用 `sh -s -- --force`。

### Windows

在 **PowerShell** 中执行：

```powershell
irm https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.ps1 | iex
```

或者在 **CMD** 中执行：

```bat
curl.exe -fsSL https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.cmd -o install.cmd && install.cmd && del install.cmd
```

脚本自动选择 x64/ARM64，校验 SHA-256，并安装到 `%LOCALAPPDATA%\Programs\unmus`，加入当前用户的 `PATH`，不需要管理员权限。重新打开终端后即可运行 `unmus`。已有程序默认不覆盖；升级、自定义位置等选项见[高级安装说明](docs/publishing.md#windows-安装器选项)。

也可以从 [Releases](https://github.com/LeifWebber/music-unlock/releases) 下载 ZIP 手动解压。

### 不安装，直接运行

macOS / Linux 可以只下载并运行一次，结束后自动清理程序：

```sh
curl -fsSL https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.sh \
  | sh -s -- --run
```

这会自动发现本机下载的音乐，也可在 `--run` 后传入目标目录，或同时指定源文件和目标目录：

```sh
curl -fsSL https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.sh \
  | sh -s -- --run "歌曲.ncm" "已转换"
```

Windows PowerShell 一次性运行：

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/LeifWebber/music-unlock/main/scripts/install.ps1))) -Run
```

需要指定路径时，在末尾加 `-Arguments @("源文件或目录", "输出目录")`；只有一个路径时仍表示目标目录。一次性运行不安装程序、不修改 `PATH`，结束后清理临时程序。

所有方式均无需安装 Go、Python、Node.js 或 Rust。

## 开始转换

直接处理本机下载的音乐：

```sh
unmus
```

程序会显示发现的目录，再开始递归转换。结果统一保存在当前系统的**下载文件夹**内，按音乐平台分目录，原文件保留。macOS 上默认是：

```text
~/Downloads/已解锁音乐/QQ音乐/
~/Downloads/已解锁音乐/网易云音乐/
```

Windows 会使用系统实际的下载文件夹位置（支持重定向到其他磁盘）；Linux 会遵循 XDG 下载目录设置。

希望把结果统一放到指定位置时，只传一个**输出目录**：

```sh
unmus "已转换"
```

也可以用两个路径，明确指定输入和输出：

```sh
unmus "歌曲.ncm" "已转换"
unmus "我的音乐" "已转换"
```

| 路径参数 | 来源 | 输出 |
| --- | --- | --- |
| 不传 | 自动发现的音乐下载目录 | 系统“下载”文件夹内的“已解锁音乐/平台” |
| 一个 | 自动发现的下载目录 | 指定目录，按平台分子目录 |
| 两个 | 第一个路径，接受文件或文件夹 | 第二个路径，直接保留相对目录结构 |

**NCM 完全离线转换，无需网易云客户端或登录。** 部分 QQ 音乐文件需要歌曲密钥，请先在本机 QQ 音乐登录能够播放该歌曲的账号；Windows 下还需保持 QQ 音乐运行。

自动发现支持 macOS/Windows 的常见下载位置，Windows 会识别系统“音乐”目录的重定向。下载位置可以在客户端修改，程序无法识别某些版本的自定义配置时，请用两个路径显式指定来源。不会扫描整个磁盘。

Windows PowerShell 示例：

```powershell
unmus
unmus "D:\Music\已转换"
unmus "C:\Music\QQ音乐" "D:\Music\已转换"
```

路径含空格时请加引号。转换失败的文件会单独列出，不影响其他歌曲继续处理。

## 支持哪些文件？

| 文件 | 通常输出为 |
| --- | --- |
| 网易云 `.ncm` | FLAC 或 MP3 |
| QQ 音乐 `.mflac`、`.qmcflac` | FLAC |
| `.mgg`、`.qmcogg` | OGG |
| `.qmc0`、`.qmc3` | MP3 |

同时支持部分带后缀的 MFLAC/MGG 变体。最终输出格式由音频内容决定。

这是本地文件转换工具，不提供歌曲下载；暂不支持酷狗 KGM 等其他格式。某些客户端版本使用不同的加密方式，可能暂时无法转换。

下表仅描述 QQ 音乐的自动取钥能力，NCM 在所有支持的系统上均可离线转换。

| 系统 | 自动获取歌曲密钥 |
| --- | --- |
| macOS | 支持读取本机 QQ 音乐当前登录态；已验证 11.7.0.2 |
| Windows | 已实现配置、会话文件和进程读取；客户端兼容性仍需实机验证 |
| Linux | 支持离线转换；需要外部密钥的文件建议在 Windows/macOS 上处理 |

## 常见问题

**需要安装 QQ 音乐吗？**

NCM 和文件本身包含密钥的 QQ 音乐格式，不需要。新版文件可能不携带密钥，此时需要本机 QQ 音乐的有效登录态和网络；程序会自动判断，不必添加额外模式参数。

**会把音乐上传到服务器吗？**

不会。转换在本地完成。需要密钥时，仅向 QQ 音乐发送认证信息和对应的歌曲标识，不上传音频、不保存登录令牌，也不切换账号。

**只想离线处理怎么办？**

```sh
unmus "我的音乐" "已转换" --offline
```

这会禁止提取登录凭据和联网；自动发现仍可读取下载目录设置。需要外部密钥的文件会说明原因并跳过转换。

**可以先看看哪些文件能转换吗？**

```sh
unmus --dry-run
```

只做离线检查，不写入输出。需要登录态才能获取密钥的文件，会提示缺少密钥。

**提示无法获取密钥怎么办？**

先确认当前 QQ 音乐账号能够播放该歌曲；登录已过期时，在客户端重新登录后再运行。Windows 用户还需确认 QQ 音乐正在运行，且与终端使用相同的 Windows 用户和权限。如果仍失败，请在 [Issues](https://github.com/LeifWebber/music-unlock/issues) 提供系统、客户端版本、文件扩展名和错误提示，勿附登录令牌或密钥。

**会损失音质吗？**

解密不会重新压缩音频。NCM 中的歌曲名、歌手、专辑及内嵌封面会写入结果；不会联网补齐文件中缺失的封面。QQ 音乐的原有标签和封面随音频保留。程序还原文件原本的音频格式，不负责转码；

**已有输出会被覆盖吗？**

不会。相同输出名称已存在时默认跳过。请注意，跳过仅表示文件存在，并不代表该文件已经通过音频完整性检查。

## 更多

- [高级用法、手工密钥与格式限制](docs/advanced.md)
- [从源码构建与参与开发](docs/development.md)
- [维护者发布指南](docs/publishing.md)

MIT License。解密算法参考 Unlock Music，完整来源和授权见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
