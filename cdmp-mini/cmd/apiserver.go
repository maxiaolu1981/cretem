package main

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver"
)

func main() {
	// 初始化日志后立即输出一条Info日志
	apiserver.NewApp("iam-apiserver").Run()
}
