# Makefile
VERSION := v1.0.1
GIT_COMMIT := $(shell git rev-parse HEAD)
BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_TREE_STATE := $(shell if git diff --quiet; then echo "clean"; else echo "dirty"; fi)

build:
	go build -ldflags " \
	  -X 'github.com/maxiaolu1981/cretem/nexuscore/component-base/version.GitVersion=$(VERSION)' \
	  -X 'github.com/maxiaolu1981/cretem/nexuscore/component-base/version.GitCommit=$(GIT_COMMIT)' \
	  -X 'github.com/maxiaolu1981/cretem/nexuscore/component-base/version.BuildDate=$(BUILD_DATE)' \
	  -X 'github.com/maxiaolu1981/cretem/nexuscore/component-base/version.GitTreeState=$(GIT_TREE_STATE)' \
	" -o iam-apiserver ./