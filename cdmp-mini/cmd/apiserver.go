package main

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver"
)

func main() {
	apiserver.NewApp("iam-server").Run()
}
