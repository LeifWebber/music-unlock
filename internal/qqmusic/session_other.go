//go:build !darwin && !windows

package qqmusic

import (
	"context"
	"errors"
)

func LoadSession(context.Context) (Credentials, error) {
	return Credentials{}, errors.New("此文件需要歌曲密钥；请在安装 QQ 音乐的 Windows/macOS 电脑上转换，或通过 --key-file 导入密钥")
}
