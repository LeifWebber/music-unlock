package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/LeifWebber/music-unlock/internal/discovery"
)

type job struct {
	source, output, label string
	directory             bool
}
type discoverSources func() ([]discovery.Source, error)

func plan(args []string, discover discoverSources, defaultOutput func() (string, error)) ([]job, error) {
	var sources []discovery.Source
	if len(args) == 2 {
		sources = []discovery.Source{{Path: args[0]}}
	} else {
		var err error
		sources, err = discover()
		if err != nil {
			return nil, err
		}
		if len(sources) == 0 {
			return nil, errors.New("未找到 QQ 音乐或网易云音乐的下载目录；请使用 unmus <源文件或文件夹> <输出目录> 指定路径")
		}
	}
	var destination string
	switch len(args) {
	case 2:
		destination = args[1]
	case 1:
		destination = args[0]
	default:
		var err error
		destination, err = defaultOutput()
		if err != nil {
			return nil, fmt.Errorf("无法定位系统下载文件夹，请显式指定输出目录: %w", err)
		}
	}
	output, err := resolveOutput(destination)
	if err != nil {
		return nil, err
	}
	var jobs []job
	for _, s := range sources {
		source, err := filepath.Abs(s.Path)
		if err != nil {
			return nil, err
		}
		source, err = filepath.EvalSymlinks(source)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(source)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil, errors.New("输入必须是普通文件或目录")
		}
		if info.IsDir() && source == output {
			return nil, errors.New("输出目录不能等于输入目录")
		}
		jobs = append(jobs, job{source, output, s.Platform, info.IsDir()})
	}
	return jobs, nil
}
