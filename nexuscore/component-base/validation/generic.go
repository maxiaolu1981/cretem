/*
包摘要
该包 validation 提供了一系列数据验证工具函数，用于校验各类输入数据的合法性，包括用户名、标签值、DNS 格式、端口号、IP 地址、百分比格式及密码等。验证逻辑基于正则表达式和规则判断，返回具体的错误信息，适用于需要严格数据校验的场景（如 API 接口参数验证）。
核心功能与流程
包内主要通过正则匹配和规则校验实现数据验证，核心流程如下：
定义校验规则：针对不同数据类型（如用户名、密码、DNS 标签等），定义正则表达式（如 qualifiedNameRegexp 用于用户名校验）和长度限制（如密码长度 8-16 位）。
实现验证函数：每个函数接收待验证值，通过正则匹配或逻辑判断检查是否符合规则，返回错误信息列表（或单个错误）。
错误信息处理：通过辅助函数（如 RegexError、MaxLenError）生成标准化的错误提示，明确告知校验失败原因


*/

// Package validation 提供各类数据合法性校验工具，用于验证用户名、密码、DNS格式等输入数据。
package validation

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"unicode"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation/field"
)

// 用户名相关校验规则
const (
	qnameCharFmt     string = "[A-Za-z0-9]"                                               // 用户名允许的基础字符（字母、数字）
	qnameExtCharFmt  string = "[-A-Za-z0-9_.]"                                            // 用户名允许的扩展字符（含-、_、.）
	qualifiedNameFmt string = "(" + qnameCharFmt + qnameExtCharFmt + "*)?" + qnameCharFmt // 用户名完整格式
)

const (
	qualifiedNameErrMsg    string = "必须由字母、数字、'-'、'_'或'.'组成，且首尾必须为字母或数字"
	qualifiedNameMaxLength int    = 63 // 用户名最大长度
)

// 用户名校验正则表达式（匹配qualifiedNameFmt）
var qualifiedNameRegexp = regexp.MustCompile("^" + qualifiedNameFmt + "$")

// IsQualifiedName 验证字符串是否为合法的"合格名称"（支持可选DNS前缀，如"example.com/MyName"）
// 返回错误信息列表（空列表表示验证通过）
func IsQualifiedName(value string) []string {
	var errs []string
	parts := strings.Split(value, "/") // 按"/"分割前缀和名称部分
	var name string

	switch len(parts) {
	case 1:
		name = parts[0] // 无前缀，直接使用整个值作为名称
	case 2:
		var prefix string
		prefix, name = parts[0], parts[1] // 提取前缀和名称
		if len(prefix) == 0 {
			errs = append(errs, "前缀部分"+EmptyError()) // 前缀不能为空
		} else if msgs := IsDNS1123Subdomain(prefix); len(msgs) != 0 {
			errs = append(errs, prefixEach(msgs, "前缀部分")...) // 前缀需符合DNS子域名规则
		}
	default:
		// 分割后超过2部分，不符合格式
		return append(
			errs,
			"合格名称"+RegexError(
				qualifiedNameErrMsg,
				qualifiedNameFmt,
				"MyName",
				"my.name",
				"123-abc",
			)+"，可包含可选DNS前缀（如'example.com/MyName'）",
		)
	}

	// 验证名称部分
	if len(name) == 0 {
		errs = append(errs, "名称部分"+EmptyError()) // 名称不能为空
	} else if len(name) > qualifiedNameMaxLength {
		errs = append(errs, "名称部分"+MaxLenError(qualifiedNameMaxLength)) // 名称长度超限
	}
	if !qualifiedNameRegexp.MatchString(name) {
		errs = append(
			errs,
			"名称部分"+RegexError(qualifiedNameErrMsg, qualifiedNameFmt, "MyName", "my.name", "123-abc"),
		)
	}
	return errs
}

// 标签值相关校验规则
const labelValueFmt string = "(" + qualifiedNameFmt + ")?" // 标签值格式（允许为空或符合用户名规则）

const labelValueErrMsg string = "标签值必须为空或由字母、数字、'-'、'_'或'.'组成，且首尾必须为字母或数字"

// LabelValueMaxLength 标签值最大长度
const LabelValueMaxLength int = 63

// 标签值校验正则表达式
var labelValueRegexp = regexp.MustCompile("^" + labelValueFmt + "$")

// IsValidLabelValue 验证字符串是否为合法的标签值
func IsValidLabelValue(value string) []string {
	var errs []string
	if len(value) > LabelValueMaxLength {
		errs = append(errs, MaxLenError(LabelValueMaxLength)) // 长度超限
	}
	if !labelValueRegexp.MatchString(value) {
		errs = append(errs, RegexError(labelValueErrMsg, labelValueFmt, "MyValue", "my_value", "12345"))
	}
	return errs
}

// DNS 标签（如域名中的一段）校验规则
const dns1123LabelFmt string = "[a-z0-9]([-a-z0-9]*[a-z0-9])?" // 小写字母、数字、-，首尾必须为字母/数字

const dns1123LabelErrMsg string = "DNS-1123标签必须由小写字母、数字或'-'组成，且首尾必须为字母或数字"

// DNS1123LabelMaxLength DNS标签最大长度
const DNS1123LabelMaxLength int = 63

// DNS标签校验正则表达式
var dns1123LabelRegexp = regexp.MustCompile("^" + dns1123LabelFmt + "$")

// IsDNS1123Label 验证字符串是否符合DNS-1123标签格式（如域名中的单个标签）
func IsDNS1123Label(value string) []string {
	var errs []string
	if len(value) > DNS1123LabelMaxLength {
		errs = append(errs, MaxLenError(DNS1123LabelMaxLength))
	}
	if !dns1123LabelRegexp.MatchString(value) {
		errs = append(errs, RegexError(dns1123LabelErrMsg, dns1123LabelFmt, "my-name", "123-abc"))
	}
	return errs
}

// DNS子域名校验规则
const dns1123SubdomainFmt string = dns1123LabelFmt + "(\\." + dns1123LabelFmt + ")*" // 多个DNS标签用.连接

const dns1123SubdomainErrorMsg string = "DNS-1123子域名必须由小写字母、数字、'-'或'.'组成，且首尾必须为字母或数字"

// DNS1123SubdomainMaxLength DNS子域名最大长度
const DNS1123SubdomainMaxLength int = 253

// DNS子域名校验正则表达式
var dns1123SubdomainRegexp = regexp.MustCompile("^" + dns1123SubdomainFmt + "$")

// IsDNS1123Subdomain 验证字符串是否符合DNS-1123子域名格式（如"example.com"）
func IsDNS1123Subdomain(value string) []string {
	var errs []string
	if len(value) > DNS1123SubdomainMaxLength {
		errs = append(errs, MaxLenError(DNS1123SubdomainMaxLength))
	}
	if !dns1123SubdomainRegexp.MatchString(value) {
		errs = append(errs, RegexError(dns1123SubdomainErrorMsg, dns1123SubdomainFmt, "example.com"))
	}
	return errs
}

// IsValidPortNum 验证端口号是否合法（1-65535之间）
func IsValidPortNum(port int) []string {
	if 1 <= port && port <= 65535 {
		return nil
	}
	return []string{InclusiveRangeError(1, 65535)}
}

// IsInRange 验证数值是否在[min, max]范围内
func IsInRange(value int, min int, max int) []string {
	if value >= min && value <= max {
		return nil
	}
	return []string{InclusiveRangeError(min, max)}
}

// IsValidIP 验证字符串是否为合法IP地址（IPv4或IPv6）
func IsValidIP(value string) []string {
	if net.ParseIP(value) == nil {
		return []string{"必须是有效的IP地址（如10.9.8.7）"}
	}
	return nil
}

// IsValidIPv4Address 验证字符串是否为合法IPv4地址
func IsValidIPv4Address(fldPath *field.Path, value string) field.ErrorList {
	var allErrors field.ErrorList
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil { // To4()返回nil表示非IPv4
		allErrors = append(allErrors, field.Invalid(fldPath, value, "必须是有效的IPv4地址"))
	}
	return allErrors
}

// IsValidIPv6Address 验证字符串是否为合法IPv6地址
func IsValidIPv6Address(fldPath *field.Path, value string) field.ErrorList {
	var allErrors field.ErrorList
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() != nil { // To4()非nil表示是IPv4
		allErrors = append(allErrors, field.Invalid(fldPath, value, "必须是有效的IPv6地址"))
	}
	return allErrors
}

// 百分比格式校验规则
const (
	percentFmt    string = "[0-9]+%" // 数字+%格式（如10%）
	percentErrMsg string = "有效的百分比格式必须是数字后跟'%'（如1%、93%）"
)

// 百分比校验正则表达式
var percentRegexp = regexp.MustCompile("^" + percentFmt + "$")

// IsValidPercent 验证字符串是否为合法的百分比格式（如"50%"）
func IsValidPercent(percent string) []string {
	if !percentRegexp.MatchString(percent) {
		return []string{RegexError(percentErrMsg, percentFmt, "1%", "93%")}
	}
	return nil
}

// MaxLenError 生成"长度超限"的错误提示
func MaxLenError(length int) string {
	return fmt.Sprintf("长度不能超过%d个字符", length)
}

// RegexError 生成正则匹配失败的错误提示，包含示例和正则规则
func RegexError(msg string, fmt string, examples ...string) string {
	if len(examples) == 0 {
		return msg + "（用于验证的正则表达式：'" + fmt + "'）"
	}
	msg += "（例如："
	for i := range examples {
		if i > 0 {
			msg += "或"
		}
		msg += "'" + examples[i] + "'，"
	}
	msg += "用于验证的正则表达式：'" + fmt + "'）"
	return msg
}

// EmptyError 生成"不能为空"的错误提示
func EmptyError() string {
	return "不能为空"
}

// prefixEach 给错误信息列表中的每个信息添加前缀
func prefixEach(msgs []string, prefix string) []string {
	for i := range msgs {
		msgs[i] = prefix + msgs[i]
	}
	return msgs
}

// InclusiveRangeError 生成"数值范围"的错误提示（包含min和max）
func InclusiveRangeError(lo, hi int) string {
	return fmt.Sprintf("必须在%d到%d之间（包含边界）", lo, hi)
}

// 密码校验规则
const (
	minPassLength = 8  // 密码最小长度
	maxPassLength = 16 // 密码最大长度
)

// IsValidPassword 验证密码是否合法，返回具体错误信息
func IsValidPassword(password string) error {
	var hasUpper bool      // 是否包含大写字母
	var hasLower bool      // 是否包含小写字母
	var hasNumber bool     // 是否包含数字
	var hasSpecial bool    // 是否包含特殊字符（标点或符号）
	var passLen int        // 密码长度
	var errorString string // 错误信息汇总

	// 遍历密码字符，检查各类字符是否存在
	for _, ch := range password {
		switch {
		case unicode.IsNumber(ch):
			hasNumber = true
			passLen++
		case unicode.IsUpper(ch):
			hasUpper = true
			passLen++
		case unicode.IsLower(ch):
			hasLower = true
			passLen++
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
			passLen++
		case ch == ' ': // 允许空格
			passLen++
		}
	}

	// 拼接错误信息
	appendError := func(err string) {
		if len(strings.TrimSpace(errorString)) != 0 {
			errorString += "，" + err
		} else {
			errorString = err
		}
	}
	if !hasLower {
		appendError("缺少小写字母")
	}
	if !hasUpper {
		appendError("缺少大写字母")
	}
	if !hasNumber {
		appendError("至少需要一个数字")
	}
	if !hasSpecial {
		appendError("缺少特殊字符")
	}
	if !(minPassLength <= passLen && passLen <= maxPassLength) {
		appendError(
			fmt.Sprintf("密码长度必须在%d到%d个字符之间", minPassLength, maxPassLength),
		)
	}

	if len(errorString) != 0 {
		return fmt.Errorf("%s", errorString)
	}

	return nil
}
