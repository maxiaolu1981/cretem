#!/bin/bash
# get_token.sh - 单独获取管理员令牌的脚本

# 配置
IAM_API_URL="http://127.0.0.1:8080"
 #ADMIN_USER="admin"
 #ADMIN_PASSWORD="Admin@2021"
ADMIN_USER="gettest-user104"
ADMIN_PASSWORD="TestPass123!"


# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }

# 获取令牌并打印
log_info "尝试获取令牌: ${IAM_API_URL}/login"
response=$(curl -s -w "\n%{http_code}" -X POST "${IAM_API_URL}/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"'"${ADMIN_USER}"'","password":"'"${ADMIN_PASSWORD}"'"}')

http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n -1)

log_info "登录状态码: $http_code"
log_info "响应体: $body"

if [ "$http_code" -ne 200 ]; then
    log_error "登录失败，状态码: $http_code"
    exit 1
fi

# 提取令牌
token=$(echo "$body" | awk -F'"token":"' '{print $2}' | awk -F'"' '{print $1}')

if [ -z "$token" ] || [ ${#token} -lt 20 ]; then
    log_error "令牌解析失败，提取结果: [$token]"
    exit 1
fi

log_success "令牌获取成功（请复制以下令牌用于下一步）:"
echo -e "\n${GREEN}${token}${NC}\n"
log_info "提示: 复制令牌时确保没有多余空格"