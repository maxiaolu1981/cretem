#!/bin/bash
# validate-api.sh - 验证API服务存活状态及登录接口功能

# 配置参数
API_HOST="localhost"
API_PORT="8080"
HEALTH_CHECK_ENDPOINT="/healthz"
LOGIN_ENDPOINT="/login"
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
        echo "响应内容: ${login_response}"
    else
        echo -e "${YELLOW}⚠️ 登录响应格式异常${NC}"
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

print_separator
echo -e "${GREEN}所有API验证通过${NC}"
exit 0