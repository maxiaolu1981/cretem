package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func isExecuteable(filePath string) bool {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	if !fileInfo.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		ext := path.Ext(filePath)
		if ext == ".exe" || ext == ".bat" || ext == ".com" {
			return true
		}
	}
	return fileInfo.Mode()&0111 != 0
}

func findExefile(file string) (string, error) {
	if strings.TrimSpace(file) == "" {
		return "", errors.WrapC(fmt.Errorf("%s文件不能为空", file), code.ErrValidation, "文件不能为空")
	}
	if path.IsAbs(file) || file[0] == '.' {
		if isExecuteable(file) {
			return filepath.Abs(file)
		}
		return "", fmt.Errorf("文件%s不存在", file)
	}
	path := os.Getenv("PATH")
	if path == "" {
		return "", errors.Errorf("环境变量PATH为空")
	}
	pathList := filepath.SplitList(path)
	for _, v := range pathList {
		fullPath := filepath.Join(v, file)
		if !isExecuteable(fullPath) {
			continue
		}
		return fullPath, nil
	}
	return "", fmt.Errorf("在path中未找到可执行文件%s", file)
}

func main() {
	//if len(os.Args) < 2
}
