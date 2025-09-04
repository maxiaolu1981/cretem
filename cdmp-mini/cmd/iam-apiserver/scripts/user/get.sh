#!/bin/bash
set -euo pipefail

# ==================== 配置区域 - 完全适配当前服务端返回 ====================
# API基础配置
BASE_URL="http://localhost:8080"          # 完整基础URL（不可缺少http://）
API_VERSION="v1"                          # API版本

# 测试用户数据（与数据库一致）
VALID_USER="gettest-user101"              # 普通用户（IsAdmin=0，存在）
ADMIN_USER="admin"                        # 管理员用户（IsAdmin=1，存在，用于测试权限）
INVALID_USER="non-existent-user-999"      # 不存在的用户（用于404测试）
INVALID_ROUTE_PATH="invalid-route-123"    # 完全不存在的路由（非/users路径下）

# 认证Token（均为签名有效、sub/identity正确的Token）
# 1. 管理员Token（查所有用户有权限）
VALID_TOKEN="Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzA3MTc3OCwiaWRlbnRpdHkiOiJhZG1pbiIsImlzcyI6Imh0dHBzOi8vZ2l0aHViLmNvbS9tYXhpYW9sdTE5ODEvY3JldGVtIiwib3JpZ19pYXQiOjE3NTY5ODUzNzgsInN1YiI6ImFkbWluIn0.1JYTDvhxwOFvL3GTR73bCSqtcT1QhK3hK9uvcntLbVY"
# 2. 普通用户Token（sub=gettest-user101，IsAdmin=0）
NO_PERMISSION_TOKEN="Bearer eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1Njk5ODkzNiwiaWRlbnRpdHkiOiJnZXR0ZXN0LXVzZXIxMDEiLCJpc3MiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsIm9yaWdpbl9pYXQiOjE3NTY5OTg5MzYsInN1YiI6ImdldHRlc3QtdXNlcjEwMSJ9.sGW3Q41zz7ZH8p0TBsAZqbIKp2u7SVmnC_51m9src0g"
# 3. 过期Token（格式正确，已过期）
EXPIRED_TOKEN="Bearer eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1Njk4MzE2MywiaWRlbnRpdHkiOiJhZG1pbiIsImlzcyI6Imh0dHBzOi8vZ2l0aHViLmNvbS9tYXhpYW9sdTE5ODEvY3JldGVtIiwib3JpZ2luX2lhdCI6MTc1Njk4MzE2Mywic3ViIjoiZ2V0dGVzdC11c2VyMTAxIn0.teom6K1tEciPlVbKqDIXCVRb4r-EJZhdtDSBoanrrds"
# 4. 格式无效Token（无Bearer前缀）
INVALID_FORMAT_TOKEN="invalid_token_without_bearer"
# 5. 内容无效Token（格式正确，签名错误）
INVALID_CONTENT_TOKEN="Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VybmFtZSI6ImJhc2ljX3Rva2VuXzEyMyJ9.xxx"

# 业务错误码（完全匹配服务端实际返回）
ERR_SUCCESS_CODE=0                        # 成功（200）
ERR_USER_NOT_FOUND=110001                 # 用户不存在（404）
ERR_TOKEN_BASE64_INVALID=100208           # Token Base64无效（401）
ERR_TOKEN_EXPIRED=110003                  # Token过期（401，服务端实际返回）
ERR_MISSING_HEADER=100205                 # 缺少Authorization头（401）
ERR_INVALID_HEADER=100204                 # Authorization格式无效（400）
ERR_PERMISSION_DENIED=100207              # 无权限（403）
ERR_ROUTE_NOT_FOUND=100301                # 路由不存在（404，服务端修正后）
ERR_PAGE_NOT_FOUND=100005    # 对应服务端的code.ErrPageNotFound

# 颜色定义（终端彩色输出）
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # 重置颜色

# 测试计数器
total_case=0
passed_case=0
failed_case=0

# ==================== 依赖检查函数 ====================
# 检查jq工具（解析JSON必需）
check_jq() {
    if ! command -v jq &> /dev/null; then
        echo -e "\n${RED}❌ 错误：未安装jq工具（解析JSON响应必需）${NC}"
        echo -e "${YELLOW}👉 安装命令：${NC}"
        echo -e "   Ubuntu/Debian: sudo apt update && sudo apt install -y jq"
        echo -e "   CentOS/RHEL: sudo yum install -y jq"
        exit 1
    fi
}

# 检查核心Token配置（避免基础错误）
check_core_token() {
    # 检查VALID_TOKEN
    if [ -z "$VALID_TOKEN" ] || [[ "$VALID_TOKEN" != Bearer* ]]; then
        echo -e "\n${RED}❌ 错误：VALID_TOKEN格式无效（需为 Bearer <三段式Token>）${NC}"
        exit 1
    fi
    # 检查NO_PERMISSION_TOKEN
    if [ -z "$NO_PERMISSION_TOKEN" ] || [[ "$NO_PERMISSION_TOKEN" != Bearer* ]]; then
        echo -e "\n${RED}❌ 错误：NO_PERMISSION_TOKEN格式无效（需为 Bearer <三段式Token>）${NC}"
        exit 1
    fi
}

# ==================== 核心测试函数 ====================
# 功能：执行单个测试用例，包含请求发送、结果解析、多维度校验
run_test() {
    local case_name="$1"
    local req_url="$2"
    local req_token="$3"
    local exp_http="$4"
    local exp_code="$5"
    
    # 计数器自增
    total_case=$((total_case + 1))
    
    # 输出用例基础信息
    echo -e "\n${PURPLE}==================================================${NC}"
    echo -e "${CYAN}测试用例 $total_case：$case_name${NC}"
    echo -e "${BLUE}请求URL：$req_url${NC}"
    echo -e "${BLUE}预期结果：HTTP $exp_http | 错误码 $exp_code${NC}"
    [ -n "$req_token" ] && echo -e "${BLUE}Token预览：${req_token:0:25}...${NC}" || echo -e "${BLUE}Token：无${NC}"
    
    # 发送HTTP请求（带超时和错误捕获）
    local response
    if [ -z "$req_token" ]; then
        response=$(curl -s -S -w "\n%{http_code}" \
            --max-time 10 \
            -H "Content-Type: application/json" \
            "$req_url" 2>&1)
    else
        response=$(curl -s -S -w "\n%{http_code}" \
            --max-time 10 \
            -H "Authorization: $req_token" \
            -H "Content-Type: application/json" \
            "$req_url" 2>&1)
    fi
    
    # 分离响应体和HTTP状态码（处理curl错误）
    local http_code=$(echo "$response" | tail -n1)
    local response_body=$(echo "$response" | head -n -1)
    
    # 处理curl直接报错的情况（如URL格式错误）
    if echo "$response_body" | grep -q "curl: ("; then
        failed_case=$((failed_case + 1))
        echo -e "${RED}❌ 测试失败：请求发送失败${NC}"
        echo -e "${RED}错误信息：$response_body${NC}"
        return
    fi
    
    # 输出实际响应
    echo -e "${YELLOW}实际结果：HTTP $http_code${NC}"
    echo -e "${YELLOW}响应内容：${NC}"
    if echo "$response_body" | jq . &> /dev/null; then
        echo "$response_body" | jq .
    else
        echo "$response_body"
    fi
    
    # 1. 校验HTTP状态码
    if [ "$http_code" -ne "$exp_http" ]; then
        failed_case=$((failed_case + 1))
        echo -e "${RED}❌ 测试失败：HTTP状态码不匹配${NC}"
        echo -e "${RED}   预期：$exp_http | 实际：$http_code${NC}"
        return
    fi
    
    # 2. 成功响应（200）特殊校验
    if [ "$exp_http" -eq 200 ]; then
        # 校验data字段存在
        if ! echo "$response_body" | jq '.data' &> /dev/null; then
            failed_case=$((failed_case + 1))
            echo -e "${RED}❌ 测试失败：成功响应缺少data字段${NC}"
            return
        fi
        # 校验用户名匹配（查询用户场景）
        if [[ "$req_url" == *"/users/"* ]]; then
            local actual_user=$(echo "$response_body" | jq -r '.data.Username')
            local target_user=$(echo "$req_url" | awk -F '/users/' '{print $2}')
            if [ "$actual_user" != "$target_user" ]; then
                failed_case=$((failed_case + 1))
                echo -e "${RED}❌ 测试失败：返回用户名与请求目标不匹配${NC}"
                echo -e "${RED}   预期：$target_user | 实际：$actual_user${NC}"
                return
            fi
        fi
        # 校验成功码为0
        local actual_code=$(echo "$response_body" | jq -r '.code')
        if [ "$actual_code" -ne "$ERR_SUCCESS_CODE" ]; then
            failed_case=$((failed_case + 1))
            echo -e "${RED}❌ 测试失败：成功响应code错误${NC}"
            echo -e "${RED}   预期：$ERR_SUCCESS_CODE | 实际：$actual_code${NC}"
            return
        fi
        # 成功通过
        passed_case=$((passed_case + 1))
        echo -e "${GREEN}✅ 测试通过${NC}"
        return
    fi
    
    # 3. 错误响应（非200）校验
    # 校验code字段存在
    if ! echo "$response_body" | jq '.code' &> /dev/null; then
        failed_case=$((failed_case + 1))
        echo -e "${RED}❌ 测试失败：错误响应缺少code字段${NC}"
        return
    fi
    # 校验错误码匹配
    local actual_code=$(echo "$response_body" | jq -r '.code')
    if [ "$actual_code" -ne "$exp_code" ]; then
        failed_case=$((failed_case + 1))
        echo -e "${RED}❌ 测试失败：错误码不匹配${NC}"
        echo -e "${RED}   预期：$exp_code | 实际：$actual_code${NC}"
        return
    fi
    # 校验message字段存在
    if ! echo "$response_body" | jq '.message' &> /dev/null; then
        failed_case=$((failed_case + 1))
        echo -e "${RED}❌ 测试失败：错误响应缺少message字段${NC}"
        return
    fi
    
    # 错误响应校验通过
    passed_case=$((passed_case + 1))
    echo -e "${GREEN}✅ 测试通过${NC}"
}

# ==================== 主测试流程 ====================
main() {
    # 前置依赖检查
    check_jq
    check_core_token
    
    # 1. 构建所有测试URL（确保完整、无相对路径）
    local valid_user_url="${BASE_URL}/${API_VERSION}/users/${VALID_USER}"       # 普通用户查询（自己）
    local admin_user_url="${BASE_URL}/${API_VERSION}/users/${ADMIN_USER}"       # 管理员查询（测试权限）
    local invalid_user_url="${BASE_URL}/${API_VERSION}/users/${INVALID_USER}"   # 不存在用户查询
    local invalid_route_url="${BASE_URL}/${API_VERSION}/${INVALID_ROUTE_PATH}"  # 完全无效路由（非/users）
    
    # 输出测试启动信息
    echo -e "${CYAN}===== 开始执行用户API自动化测试 =====${NC}"
    echo -e "${BLUE}基础URL：$BASE_URL/${API_VERSION}${NC}"
    echo -e "${BLUE}测试用户：普通用户=$VALID_USER | 管理员=$ADMIN_USER${NC}"
    echo -e "${BLUE}预计用例数：8个${NC}"
    
    # 2. 执行所有测试用例（按场景分类）
    # 用例1：管理员Token查询普通用户（成功）
    run_test "管理员查询普通用户（有权限）" \
        "$valid_user_url" \
        "$VALID_TOKEN" \
        200 \
        "$ERR_SUCCESS_CODE"
    
    # 用例2：管理员Token查询不存在用户（404）
    run_test "管理员查询不存在用户" \
        "$invalid_user_url" \
        "$VALID_TOKEN" \
        404 \
        "$ERR_USER_NOT_FOUND"
    
    # 用例3：无Token访问受保护接口（401）
    run_test "缺少Authorization请求头" \
        "$valid_user_url" \
        "" \
        401 \
        "$ERR_MISSING_HEADER"
    
    # 用例4：Token格式无效（无Bearer前缀，400）
    run_test "Authorization格式无效（无Bearer）" \
        "$valid_user_url" \
        "$INVALID_FORMAT_TOKEN" \
        400 \
        "$ERR_INVALID_HEADER"
    
    # 用例5：Token内容无效（签名错误，401）
    run_test "Token格式正确但内容无效（签名错误）" \
        "$valid_user_url" \
        "$INVALID_CONTENT_TOKEN" \
        401 \
        "$ERR_TOKEN_BASE64_INVALID"
    
    # 用例6：过期Token访问（401）
    run_test "使用过期Token查询" \
        "$valid_user_url" \
        "$EXPIRED_TOKEN" \
        401 \
        "$ERR_TOKEN_EXPIRED"
    
    # 用例7：普通用户查询管理员（无权限，403）- 核心修复
    run_test "普通用户查询管理员（无权限）" \
        "$admin_user_url" \
        "$NO_PERMISSION_TOKEN" \
        403 \
        "$ERR_PERMISSION_DENIED"
    
    # 用例8：访问完全无效的路由（404）- 核心修复
    run_test "访问不存在的路由（非/users路径）" \
        "$invalid_route_url" \
        "$VALID_TOKEN" \
        404 \
        "$ERR_PAGE_NOT_FOUND"
    
    # 3. 输出测试总结
    echo -e "\n${PURPLE}==================================================${NC}"
    echo -e "${CYAN}===== 测试总结 =====${NC}"
    echo -e "总用例数：$total_case"
    echo -e "${GREEN}通过用例：$passed_case${NC}"
    echo -e "${RED}失败用例：$failed_case${NC}"
    
    # 最终结果提示
    if [ "$failed_case" -eq 0 ]; then
        echo -e "${GREEN}🎉 所有测试用例均通过！${NC}"
    else
        echo -e "${RED}❌ 部分用例失败，请根据错误提示排查：${NC}"
        echo -e "${RED}   1. 若用例7失败：确认ADMIN_USER（$ADMIN_USER）在数据库中存在且IsAdmin=1${NC}"
        echo -e "${RED}   2. 若用例8失败：确认invalid_route_url（$invalid_route_url）是服务端未定义的路由${NC}"
    fi
}

# 启动测试
main