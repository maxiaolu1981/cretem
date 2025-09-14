package main

import (
	"math/rand"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver"
	_ "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	debug.SetGCPercent(100)       // 默认100，可根据需要调整
	debug.SetMemoryLimit(2 << 30) // 设置2GB内存限制

	// 设置GOMAXPROCS
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)

	version.CheckVersionAndExit()
	apiserver.NewApp("iam-apiserver").Run()

}
