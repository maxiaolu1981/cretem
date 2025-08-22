package main

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
)

func main() {
	version.CheckVersionAndExit()
	apiserver.NewApp("iam-apiserver").Run()
}
