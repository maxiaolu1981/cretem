package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func isExecutable(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !fileInfo.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		ext := filepath.Ext(path)
		if ext == ".bat" || ext == ".exe" || ext == ".com" {
			return true
		} else {
			return false
		}
	}
	return fileInfo.Mode()&0111 != 0
}

func findInExecutable(file string) (string, error) {
	if strings.TrimSpace(file) == "" {
		return "", errors.New("文件名不能为空")
	}
	if filepath.IsAbs(file) || file[0] == '.' {
		resolvedPath, err := filepath.Abs(file)
		if err != nil {
			return "", errors.New("绝对路径转换错误")
		}
		if isExecutable(resolvedPath) {
			return resolvedPath, nil
		}
	}
	paths := os.Getenv("PATH")
	if paths == "" {
		return "", errors.New("没有设置环境变量")
	}
	pathList := filepath.SplitList(paths)
	for _, v := range pathList {
		fullPath := filepath.Join(v, file)
		if isExecutable(fullPath) {
			return fullPath, nil
		}
	}
	return "", errors.New("没有发现可执行文件")
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "用法: %s <可执行文件名1> [可执行文件名2...]\n")
		os.Exit(1)
	}
	for _, v := range args {
		fullPath, err := findInExecutable(v)
		if err == nil {
			fmt.Println(fullPath)
		}
	}
}
