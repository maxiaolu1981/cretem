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

package main

import (
	"fmt"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/code"
	base "github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func main() {
	if err := getDatabase(); err != nil {
		//if base.IsCode(err, code.ErrTokenInvalid) {
		fmt.Printf("找到了错误%+v\n", err)
		//}
		//coder := base.ParseCode(err)
		//fmt.Printf("%+v\n", coder)
		//	base.ListAllCodes()
	}
}

func getDatabase() error {
	if err := getUser(); err != nil {
		return base.WrapC(err, code.ErrDatabase, "验证失败.")
	}
	return nil
}

func getUser() error {
	if err := queryDatabase(); err != nil {
		return base.WrapC(err, code.ErrSuccess, "正确.")
	}
	return nil
}

func queryDatabase() error {
	return base.WrapC(fmt.Errorf("asdsf"), code.ErrPasswordIncorrect, "密码无效")
}
