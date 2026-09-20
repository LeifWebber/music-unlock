# 旧版 Windows 实现

这些脚本归档保留，不参与新 CLI 的构建和发布。

旧版通过 Frida 调用 `QQMusic.exe` 内 `QQMusicCommon.dll` 的特定 32 位导出符号，
仅适配原 README 记录的 QQMusic2005.22.47.07，不能用于 macOS/Linux。
`build.bat` 是旧 Nuitka 打包方式，需要另行安装 Python、Frida、PyYAML、Nuitka。

**旧脚本会删除源加密文件、移动普通音频并清理目录。** 新 CLI 始终保留源文件。
如必须研究旧版，请使用副本、在本目录运行，配置示例为 `music.example.yaml`；
不建议将这些脚本用于日常转换。新 CLI 不读取旧配置。
