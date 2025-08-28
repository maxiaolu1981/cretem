package main

import (
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	version.CheckVersionAndExit()

}
