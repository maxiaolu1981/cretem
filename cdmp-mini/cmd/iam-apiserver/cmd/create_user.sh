#!/bin/bash
# create_user.sh - 创建IAM用户（基于成功请求格式）

set -euo pipefail

# 配置（根据实际环境修改）
IAM_API_URL="http://127.0.0.1:8080"
ADMIN_USER="admin"
ADMIN_PASSWORD="Admin@2021"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

# 获取管理员令牌
get_token() {
    local response=$(curl -s -X POST "${IAM_API_URL}/login" \
        -H "Content-Type: application/json" \
        -d '{"username":"'"${ADMIN_USER}"'","password":"'"${ADMIN_PASSWORD}"'"}')
    
    echo "$response" | grep -o '"token":"[^"]*"' | cut -d'"' -f4
}

# 创建用户
create_user() {
    local username="$1"
    local nickname="$2"
    local email="$3"
    local password="$4"

    # 验证参数
    if [ -z "$username" ] || [ -z "$email" ] || [ -z "$password" ]; then
        log_error "用户名、邮箱和密码为必填项"
        exit 1
    fi

    # 获取令牌
    local token=$(get_token)
    if [ -z "$token" ]; then
        log_error "获取管理员令牌失败"
        exit 1
    fi

    # 发送创建请求（使用成功格式）
    local response=$(curl -s -w "%{http_code}" -X POST "${IAM_API_URL}/v1/users" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${token}" \
        -d '{
            "metadata": {
                "name": "'"${username}"'"
            },
            "nickname": "'"${nickname:-$username}"'",
            "email": "'"${email}"'",
            "password": "'"${password}"'",
            "phone": ""
        }')

    # 解析响应
    local http_code="${response: -3}"
    local body="${response%???}"

    if [ "$http_code" -eq 200 ] || [ "$http_code" -eq 201 ]; then
        log_success "用户创建成功！响应: $body"
    else
        log_error "创建失败 (HTTP $http_code)，响应: $body"
        exit 1
    fi
}

# 显示用法
usage() {
    echo "用法: $0 <用户名> <昵称> <邮箱> <密码>"
    echo "示例: $0 john 'John Doe' john@example.com 'JohnPass123!'"
}

# 主逻辑
if [ $# -ne 4 ]; then
    usage
    exit 1
fi

create_user "$1" "$2" "$3" "$4"