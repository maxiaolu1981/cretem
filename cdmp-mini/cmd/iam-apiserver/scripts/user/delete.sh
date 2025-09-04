#!/bin/bash
# 修复版：用户硬删除接口全量测试脚本（400/401/404/500/204）
# 核心改进：参数传递极简化，避免顺序混乱
# 使用方法：1. 修改配置区的 TOKEN 和 VALID_USER；2. chmod +x 脚本；3. 执行脚本

# ==================== 配置区（必须修改！） ====================
API_BASE_URL="http://127.0.0.1:8080/v1/users"  # 接口地址
TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzAyMTA4MCwiaWRlbnRpdHkiOiJhZG1pbiIsImlzcyI6Imh0dHBzOi8vZ2l0aHViLmNvbS9tYXhpYW9sdTE5ODEvY3JldGVtIiwib3JpZ19pYXQiOjE3NTY5MzQ2ODAsInN1YiI6ImFkbWluIn0.8URnCUoBEM-adaeV3bMJeU9Hiazebf00-9Ws8DC5GYA"  # 你的有效令牌
VALID_USER="gettest-user1000"  # 确认：这个用户必须存在！否则会触发404
NON_EXIST_USER="non_exist_$(date +%s)"  # 随机不存在的用户（404测试）
INVALID_USER="invalid@user"   # 格式无效的用户（400测试）
TIMEOUT=10  # 超时时间
# ==============================================================

# 颜色配置
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m'

# 日志函数
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_test() { echo -e "${PURPLE}[TEST]${NC} $1"; }

# 测试统计
total=0
passed=0
failed=0

# ==================== 核心函数（参数清晰无混乱） ====================
# 函数功能：发送请求并验证状态码
# 参数说明：
# $1: 测试名称（如"400参数错误"）
# $2: 请求URL（完整地址）
# $3: 预期状态码（如400、401，必须是整数）
# $4及以后: 可选请求头（如"Authorization: Bearer xxx"）
test_case() {
    local test_name="$1"
    local url="$2"
    local expect_code="$3"
    shift 3  # 移除前3个固定参数，剩下的都是请求头
    local headers=("$@")  # 接收所有请求头

    # 统计测试用例
    total=$((total + 1))
    log_test "开始测试：$test_name（预期状态码：$expect_code）"
    log_info "请求地址：$url"

    # 构建curl命令（数组方式，避免引号问题）
    curl_cmd=(
        curl -s -w "\n%{http_code}"  # -s静默模式，-w输出状态码
        -X DELETE "$url"             # 请求方法和地址
        --max-time $TIMEOUT          # 超时时间
        -H "Content-Type: application/json"  # 固定请求头
    )
    # 添加额外请求头（如授权头）
    for h in "${headers[@]}"; do
        curl_cmd+=(-H "$h")
    done

    # 执行请求并解析结果
    response=$("${curl_cmd[@]}")
    http_code=$(echo "$response" | tail -n1)  # 最后一行是状态码
    response_body=$(echo "$response" | head -n -1)  # 前面是响应体

    # 格式化响应体（有jq则美化）
    if command -v jq &> /dev/null; then
        body=$(echo "$response_body" | jq . 2>/dev/null || echo "$response_body")
    else
        body="$response_body"
    fi

    # 验证结果
    if [ "$http_code" -eq "$expect_code" ]; then
        log_success "测试通过！实际状态码：$http_code"
        log_info "响应体：$body"
        passed=$((passed + 1))
    else
        log_error "测试失败！预期：$expect_code，实际：$http_code"
        log_error "响应体：$body"
        failed=$((failed + 1))
    fi
    echo "--------------------------------------------------"
}

# ==================== 执行测试用例（参数顺序严格对应） ====================
log_info "===== 开始用户硬删除接口测试（$(date)） ====="

# 1. 测试400：用户名为空（路径//force）+ 带有效令牌
test_case \
    "400参数错误（用户名为空）" \
    "${API_BASE_URL}//force" \
    400 \
    "Authorization: Bearer ${TOKEN}"

# 2. 测试400：用户名含特殊字符（@）+ 带有效令牌
test_case \
    "400参数错误（用户名含@）" \
    "${API_BASE_URL}/${INVALID_USER}/force" \
    400 \
    "Authorization: Bearer ${TOKEN}"

# 3. 测试401：无令牌 + 不存在的用户
test_case \
    "401未授权（无令牌）" \
    "${API_BASE_URL}/${NON_EXIST_USER}/force" \
    401

# 4. 测试401：无效令牌（乱输）+ 不存在的用户
test_case \
    "401未授权（无效令牌）" \
    "${API_BASE_URL}/${NON_EXIST_USER}/force" \
    401 \
    "Authorization: Bearer invalid_token_xxx"

# 5. 测试404：有效令牌 + 不存在的用户
test_case \
    "404用户不存在" \
    "${API_BASE_URL}/${NON_EXIST_USER}/force" \
    404 \
    "Authorization: Bearer ${TOKEN}"

# 6. 测试204：有效令牌 + 存在的用户（必须确保VALID_USER存在！）
test_case \
    "204删除成功（有效用户）" \
    "${API_BASE_URL}/${VALID_USER}/force" \
    204 \
    "Authorization: Bearer ${TOKEN}"

# 7. 测试500：需后端配合（如用户名为trigger_500），暂注释（无配合则会404）
# test_case \
#     "500服务器内部错误" \
#     "${API_BASE_URL}/trigger_500/force" \
#     500 \
#     "Authorization: Bearer ${TOKEN}"

# ==================== 测试总结 ====================
log_info "===== 测试结束 ====="
log_info "总用例：$total | 通过：$passed | 失败：$failed"

if [ $failed -eq 0 ]; then
    log_success "所有测试通过！"
    exit 0
else
    log_error "有$failed个用例失败，请检查："
    log_error "1. 配置区的VALID_USER是否真的存在？"
    log_error "2. 后端接口是否正确返回对应状态码？"
    log_error "3. 令牌是否过期（可重新获取TOKEN）？"
    exit 1
fi