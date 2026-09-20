# 高级用法

## 参数

```text
unmus [输出目录] [选项]
unmus <输入文件或文件夹> <输出目录> [选项]

--offline         不提取登录凭据、不联网
--dry-run         离线检查并显示计划，不写输出
--key-file PATH   导入手工准备的 JSON 歌曲密钥映射
--version         显示版本
-h, --help        显示帮助
```

默认自动处理内嵌密钥；缺少 musicex 密钥时，按需读取当前操作系统的 QQ 音乐登录态并请求对应歌曲密钥。
旧的 `--qqmusic` 参数仍被接受，含义与默认模式相同，不能与 `--offline` 同用。
`--dry-run` 总是离线。选项可以写在位置参数前后；以 `-` 开头的路径放在 `--` 后。

## 自动发现下载目录

零参数将所有来源统一输出到系统下载文件夹下的 `已解锁音乐/<平台>/`；一个路径表示统一输出根目录；两个路径保持显式源/目标语义。自动模式按实际文件格式区分 QQ 音乐和网易云，而不是仅依据所在目录名称。

默认输出位置：macOS 使用 `~/Downloads`；Windows 通过 Known Folder API 读取 `FOLDERID_Downloads`，支持系统目录重定向；Linux 读取 `$XDG_CONFIG_HOME/user-dirs.dirs`（默认 `~/.config/user-dirs.dirs`）中的 `XDG_DOWNLOAD_DIR`，没有有效配置时使用 `~/Downloads`。读取配置不会执行 shell 命令。无法定位系统目录时会提示显式指定输出路径，不退回来源目录。`--dry-run` 不创建输出目录。

- macOS QQ 音乐：先读取常见下载目录配置字段，再检测 `~/Music/QQ音乐`、`~/Music/QQMusic`、`~/Music/VipSongsDownload`，以及普通/沙盒 Application Support 下的 `QQMusicMac/iQmc`。不自动扫描 `iMusic` 播放缓存。
- Windows QQ 音乐：读取 `%APPDATA%/Tencent/QQMusic` 中 `QQMusicServiceConfig.ini`、`QQMusicConfig.ini` 可识别的下载路径字段；再检测系统“音乐”已知文件夹中的 `VipSongsDownload`、`QQMusic`、`QQ音乐`。使用 Known Folder API 适配目录重定向。
- 网易云：检测系统“音乐”目录中的 `网易云音乐` 和 `CloudMusic`。尚未覆盖不同版本的所有自定义下载配置。
- Linux：检测 `~/Music` 中的上述常见子目录；其他位置使用两个路径显式指定。
- 路径不存在时跳过；根目录及整个用户主目录不会因配置值而自动扫描。解析符号链接后去重，避免重复扫描嵌套来源；递归时排除本次所有输出目录。
- 当前 macOS QQ 音乐实测在沙盒 `iQmc` 保存加密下载；可识别配置字段的解析有合成测试，Windows 实机兼容性仍待验证。目录不是固定不变的，不扫描整个磁盘猜测自定义路径。
- `--offline` 和 `--dry-run` 禁止获取歌曲密钥；自动发现可读取包含下载路径设置的客户端配置，但不解析其中的登录凭据。

## 手工导入密钥

大多数 macOS/Windows 用户不需要使用 `--key-file`。它是本工具支持的交换格式，**不是 QQ 音乐自带的某个 key file**。
它适用于已有 ekey、离线处理或在其他电脑转换等高级场景。

```json
{
  "歌手/歌曲.mflac": "该文件的Base64编码ekey",
  "F0M000example.mflac": "另一首歌的Base64编码ekey"
}
```

键为相对于输入目录的路径（使用 `/`），或 musicex 尾标里的媒体文件名；值为 Base64 ekey，不能填已解密的原始 key。
单文件输入时使用该文件的文件名。相对路径匹配优先于媒体文件名。

```sh
unmus ./input ./output --offline --key-file ./keys.json
```

个人密钥、客户端缓存和音乐文件不应提交到版本库或打进安装包。

## 平台上的自动发现

- macOS：读取普通或沙盒 Preferences 目录中的 QQ 音乐偏好文件，只使用客户端 `nCurrUseId` 指定的账号。
- Windows：读取 `%APPDATA%/Tencent/QQMusic/QQMusicServiceConfig.ini` 的 `[Account] Uin`；优先从 `SetCookie.dat`、`_SetCookie.dat` 找到与该账号绑定的会话，必要时只读检查当前 Windows 用户的 `QQMusic.exe` 进程。
- Windows 读取有时间和字节数上限，不注入进程、不提权、不扫描其他应用，不把任意 Base64 字符串猜作凭据。没有发现与当前账号对应的明确记录时会报错，不尝试其他账号。
- Linux 没有对应的本地客户端适配器。可以转换内嵌密钥文件，或显式导入 ekey。

认证信息仅发送给固定的 `https://u.y.qq.com/cgi-bin/musicu.fcg`，禁止 HTTP 重定向，不保存认证信息或返回的 ekey。
这是客户端兼容接口，格式可能变化；Windows 的真实 QQ 音乐版本兼容性仍需验证。

## 网易云 NCM

支持标准 `CTENFDAM` 容器、AES 密钥封装及 NCM 流密码，输出以实际 FLAC/MP3 音频头为准。支持无元数据/无封面，以及封面分配区大于实际图片的新版本布局；不需要客户端、账号或网络。

歌曲名、歌手、专辑和内嵌 JPEG/PNG 封面回填到 FLAC 或 ID3 标签，音频帧保持不变。保留原有其他标签；少见的 ID3 版本、压缩或扩展标志会保留原标签原样，不强行合并。只有远程封面 URL 时不联网下载。不支持 `.uc` 等缓存格式或未知的新容器版本。

密钥、元数据和封面块有长度上限，AES 填充和容器偏移会校验。解密后的音频头校验不能代替全曲完整性检查。

## 格式与文件行为

支持网易云 `.ncm`；QQ 音乐支持 `.qmc0`、`.qmc3`、`.qmcflac`、`.qmcogg`、`.mflac`、`.mgg`，以及 MFLAC/MGG 的 `0`、`1`、`a`、`h`、`l`、`m` 后缀。扩展名大小写不敏感。
支持静态、Map、RC4 解密与 V1/V2 ekey、QTag。musicex 可自动请求密钥；STag 目前需要手工 ekey。
相同扩展名不保证相同加密版本；不支持所有未来变体。

- 不移动或删除源文件，不复制普通音频或歌词，不跟随递归扫描中的符号链接。
- 输出目录可以位于输入目录内部，扫描时会排除它；输入与输出目录不能相同。
- 同批次的两个输入若映射到同一输出名，会报告冲突。
- 先校验音频头，再流式写入临时文件；支持硬链接的文件系统一次性发布完整输出。
- FAT/exFAT 等卷使用独占创建后复制。正常失败或 Ctrl+C 会清理本次未完成文件；强制终止和断电可能留下文件。
- 输出根目录由 `os.Root` 约束，拒绝通过子目录符号链接写到根目录外。
- 输出格式需解密后识别，所以判断已有输出前也可能需要密钥。已有文件仅按存在与否跳过。
- 音频头检查不等于全曲验证。需要时可以使用 `ffmpeg -v error -xerror -i output.flac -f null -` 检查。

| 退出码 | 含义 |
| --- | --- |
| 0 | 匹配文件成功，或因已有输出而跳过 |
| 1 | 有文件/扫描失败，或没有支持的文件 |
| 2 | 参数、路径或密钥配置错误 |
| 130 | 用户中断 |
