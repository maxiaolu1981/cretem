#!/bin/bash
# validate-api.sh - 验证API服务存活状态、登录接口及用户列表接口功能

# 配置参数
API_HOST="localhost"
API_PORT="8080"
HEALTH_CHECK_ENDPOINT="/healthz"
LOGIN_ENDPOINT="/login"
V1_USERS_ENDPOINT="/v1/users"  # 修正语法错误（等号两侧无空格）
ADMIN_USER="admin"
ADMIN_PASS="Admin@2021"  # 请根据实际密码修改
TIMEOUT=10  # 超时时间(秒)

# 颜色输出定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # 无颜色

# 打印分隔线
print_separator() {
    echo "=============================================="
}

# 1. 验证服务是否存活
print_separator
echo "1. 验证服务是否存活: http://${API_HOST}:${API_PORT}${HEALTH_CHECK_ENDPOINT}"

health_check_url="http://${API_HOST}:${API_PORT}${HEALTH_CHECK_ENDPOINT}"

# 使用curl检查健康状态
health_response=$(curl -s -w "\n%{http_code}" -X GET "${health_check_url}" --max-time ${TIMEOUT})
health_status_code=$(echo "${health_response}" | tail -n1)
health_body=$(echo "${health_response}" | head -n -1)

if [ "${health_status_code}" -eq 200 ]; then
    echo -e "${GREEN}✅ 服务正常运行${NC}"
else
    echo -e "${RED}❌ 服务未响应${NC}"
    echo "状态码: ${health_status_code}"
    echo "响应内容: ${health_body}"
    exit 1
fi

# 2. 验证登录接口
print_separator
echo "2. 验证登录接口: http://${API_HOST}:${API_PORT}${LOGIN_ENDPOINT}"

login_url="http://${API_HOST}:${API_PORT}${LOGIN_ENDPOINT}"
login_payload=$(printf '{"username":"%s","password":"%s"}' "${ADMIN_USER}" "${ADMIN_PASS}")

# 使用curl发送登录请求（分离响应体和状态码）
temp_file=$(mktemp)
login_status_code=$(curl -s -w "%{http_code}" -o "${temp_file}" \
    -X POST "${login_url}" \
    -H "Content-Type: application/json" \
    -d "${login_payload}" \
    --max-time ${TIMEOUT})

login_response=$(cat "${temp_file}")
rm -f "${temp_file}"

# 检查HTTP状态码
if [ "${login_status_code}" -eq 200 ]; then
    # 检查响应体是否包含token（简单验证）
    if echo "${login_response}" | grep -q "token"; then
        echo -e "${GREEN}✅ 登录成功${NC}"
        echo "状态码: ${login_status_code}"
        # 提取token（假设响应格式为{"token":"xxx"}）
        TOKEN=$(echo "${login_response}" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
        if [ -z "${TOKEN}" ]; then
            echo -e "${YELLOW}⚠️ 无法从响应中提取token${NC}"
            exit 1
        fi
        echo "已获取token，准备验证用户列表接口..."
    else
        echo -e "${YELLOW}⚠️ 登录响应格式异常（未包含token）${NC}"
        echo "状态码: ${login_status_code}"
        echo "响应内容: ${login_response}"
        exit 1
    fi
else
    echo -e "${RED}❌ 登录失败${NC}"
    echo "状态码: ${login_status_code}"
    echo "响应内容: ${login_response}"
    exit 1
fi

# 3. 验证/v1/users/接口（新增，包含完整内容打印）
print_separator
echo "3. 验证用户列表接口: http://${API_HOST}:${API_PORT}${V1_USERS_ENDPOINT}"

users_url="http://${API_HOST}:${API_PORT}${V1_USERS_ENDPOINT}"

# 使用登录获取的token调用用户列表接口
temp_users_file=$(mktemp)
users_status_code=$(curl -s -w "%{http_code}" -o "${temp_users_file}" \
    -X GET "${users_url}" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${TOKEN}"  # 假设使用Bearer token认证
    --max-time ${TIMEOUT})

users_response=$(cat "${temp_users_file}")
rm -f "${temp_users_file}"

# 检查用户列表接口响应
if [ "${users_status_code}" -eq 200 ]; then
    # 简单验证响应是否为JSON格式（兼容对象或数组）
    if echo "${users_response}" | grep -qE '^\{.*\}$' || echo "${users_response}" | grep -qE '^\[.*\]$'; then
        echo -e "${GREEN}✅ 用户列表接口调用成功${NC}"
        echo "状态码: ${users_status_code}"
        # 打印完整用户列表内容
        echo -e "\n${YELLOW}用户列表完整响应内容:${NC}"
        echo "${users_response}" | python3 -m json.tool  # 使用Python内置工具格式化JSON（避免依赖jq）
    else
        echo -e "${YELLOW}⚠️ 用户列表响应格式异常（非JSON）${NC}"
        echo "状态码: ${users_status_code}"
        echo "响应内容: ${users_response}"
        exit 1
    fi
else
    echo -e "${RED}❌ 用户列表接口调用失败${NC}"
    echo "状态码: ${users_status_code}"
    echo "响应内容: ${users_response}"
    exit 1
fi

print_separator
echo -e "${GREEN}所有API验证通过${NC}"
exit 0