// Copyright (c) 2025 马晓璐
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
/*
// 该包提供了 IAM 泵服务的配置管理功能，包括配置结构定义和从选项创建配置实例的方法。
// 通过将命令行或配置文件中的选项转换为服务运行所需的配置，实现了配置的统一管理，
// 为 IAM 泵服务的初始化和运行提供必要的参数支持。
*/
package config

import "github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/options"

// 它嵌入了 options.Options 结构体，用于存储api服务最终运行所需的所有配置项。
type Config struct {
	*options.Options
}

// CreateConfigFromOptions 基于给定的 IAM 泵服务命令行或配置文件选项，
// 创建一个运行配置实例。
// 参数 opts 是从命令行或配置文件解析得到的选项结构体指针。
// 返回值是创建的配置实例指针和可能出现的错误。
func CreateConfigFromOptions(opts *options.Options) (*Config, error) {
	return &Config{opts}, nil
}
