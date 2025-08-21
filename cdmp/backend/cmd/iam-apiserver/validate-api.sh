#!/bin/bash
# validate-services.sh - 验证API、HTTPS及gRPC服务（强制禁用gRPC TLS验证）

# 配置参数
API_HOST="localhost"
HTTP_PORT="8080"
HTTPS_PORT="8443"
GRPC_HOST="localhost"
GRPC_PORT="8081"

# 接口路径
HEALTH_CHECK_ENDPOINT="/healthz"
LOGIN_ENDPOINT="/login"
V1_USERS_ENDPOINT="/v1/users"

# gRPC配置（强制禁用TLS验证）
GRPC_SERVICE="proto.Cache"          # 正确的服务名
GRPC_METHOD="ListPolicies"          # 实际存在的方法名
GRPC_TLS_DIR="/var/run/iam"         # 证书目录（仅作兼容，实际不验证）

# 认证信息
ADMIN_USER="admin"
ADMIN_PASS="Admin@2021"  # 请根据实际密码修改
TIMEOUT=10               # 超时时间(秒)

# 颜色输出定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # 无颜色

# 打印分隔线
print_separator() {
    echo "=============================================="
}

# 检查命令是否存在
check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo -e "${RED}❌ 缺少必要工具: $1，请先安装${NC}"
        exit 1
    fi
}

# 1. 验证HTTP服务是否存活
print_separator
echo "1. 验证HTTP服务是否存活: http://${API_HOST}:${HTTP_PORT}${HEALTH_CHECK_ENDPOINT}"

http_health_url="http://${API_HOST}:${HTTP_PORT}${HEALTH_CHECK_ENDPOINT}"
http_health_response=$(curl -s -w "\n%{http_code}" -X GET "${http_health_url}" --max-time ${TIMEOUT})
http_health_status=$(echo "${http_health_response}" | tail -n1)
http_health_body=$(echo "${http_health_response}" | head -n -1)

if [ "${http_health_status}" -eq 200 ]; then
    echo -e "${GREEN}✅ HTTP服务正常运行${NC}"
else
    echo -e "${RED}❌ HTTP服务未响应${NC}"
    echo "状态码: ${http_health_status}"
    echo "响应内容: ${http_health_body}"
    exit 1
fi

# 2. 验证HTTPS服务是否存活
print_separator
echo "2. 验证HTTPS服务是否存活: https://${API_HOST}:${HTTPS_PORT}${HEALTH_CHECK_ENDPOINT}"

https_health_url="https://${API_HOST}:${HTTPS_PORT}${HEALTH_CHECK_ENDPOINT}"
# 使用--insecure跳过HTTPS证书验证（测试用）
https_health_response=$(curl -s -w "\n%{http_code}" -X GET "${https_health_url}" \
    --max-time ${TIMEOUT} --insecure)
https_health_status=$(echo "${https_health_response}" | tail -n1)
https_health_body=$(echo "${https_health_response}" | head -n -1)

if [ "${https_health_status}" -eq 200 ]; then
    echo -e "${GREEN}✅ HTTPS服务正常运行${NC}"
else
    echo -e "${RED}❌ HTTPS服务未响应${NC}"
    echo "状态码: ${https_health_status}"
    echo "响应内容: ${https_health_body}"
    exit 1
fi

# 3. 验证登录接口（使用HTTPS）
print_separator
echo "3. 验证HTTPS登录接口: https://${API_HOST}:${HTTPS_PORT}${LOGIN_ENDPOINT}"

login_url="https://${API_HOST}:${HTTPS_PORT}${LOGIN_ENDPOINT}"
login_payload=$(printf '{"username":"%s","password":"%s"}' "${ADMIN_USER}" "${ADMIN_PASS}")

temp_login=$(mktemp)
login_status=$(curl -s -w "%{http_code}" -o "${temp_login}" \
    -X POST "${login_url}" \
    -H "Content-Type: application/json" \
    -d "${login_payload}" \
    --max-time ${TIMEOUT} --insecure)

login_response=$(cat "${temp_login}")
rm -f "${temp_login}"

if [ "${login_status}" -eq 200 ]; then
    if echo "${login_response}" | grep -q "token"; then
        echo -e "${GREEN}✅ 登录成功${NC}"
        TOKEN=$(echo "${login_response}" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
        if [ -z "${TOKEN}" ]; then
            echo -e "${YELLOW}⚠️ 无法提取token${NC}"
            exit 1
        fi
    else
        echo -e "${YELLOW}⚠️ 登录响应不含token${NC}"
        echo "响应: ${login_response}"
        exit 1
    fi
else
    echo -e "${RED}❌ 登录失败${NC}"
    echo "状态码: ${login_status}"
    echo "响应: ${login_response}"
    exit 1
fi

# 4. 验证HTTPS用户列表接口
print_separator
echo "4. 验证HTTPS用户列表接口: https://${API_HOST}:${HTTPS_PORT}${V1_USERS_ENDPOINT}"

users_url="https://${API_HOST}:${HTTPS_PORT}${V1_USERS_ENDPOINT}"
temp_users=$(mktemp)
users_status=$(curl -s -w "%{http_code}" -o "${temp_users}" \
    -X GET "${users_url}" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${TOKEN}" \
    --max-time ${TIMEOUT} --insecure)

users_response=$(cat "${temp_users}")
rm -f "${temp_users}"

if [ "${users_status}" -eq 200 ]; then
    if echo "${users_response}" | grep -qE '^\{.*\}$|^\[.*\]$'; then
        echo -e "${GREEN}✅ 用户列表接口调用成功${NC}"
        echo -e "\n${YELLOW}用户列表内容:${NC}"
        echo "${users_response}" | python3 -m json.tool
    else
        echo -e "${YELLOW}⚠️ 响应格式非JSON${NC}"
        exit 1
    fi
else
    echo -e "${RED}❌ 用户列表接口调用失败${NC}"
    echo "状态码: ${users_status}"
    echo "响应: ${users_response}"
    exit 1
fi

# 5. 验证gRPC服务（强制禁用TLS验证）
print_separator
echo "5. 验证gRPC服务: ${GRPC_HOST}:${GRPC_PORT} (服务: ${GRPC_SERVICE}, 方法: ${GRPC_METHOD})"

# 检查grpcurl是否安装
check_command "grpcurl"

# 强制使用--insecure禁用所有TLS验证（忽略证书和主机名）
grpc_target="${GRPC_HOST}:${GRPC_PORT}"
tls_args="--insecure"
echo -e "${YELLOW}⚠️ 已禁用gRPC TLS验证（仅测试环境使用）${NC}"

# 检查gRPC服务是否存在
grpc_services=$(grpcurl ${tls_args} -max-time ${TIMEOUT} ${grpc_target} list 2>&1)
if ! echo "${grpc_services}" | grep -q "${GRPC_SERVICE}"; then
    echo -e "${RED}❌ gRPC服务 ${GRPC_SERVICE} 不存在${NC}"
    echo "可用服务: ${grpc_services}"
    exit 1
fi

# 检查方法是否存在于服务中
grpc_methods=$(grpcurl ${tls_args} -max-time ${TIMEOUT} ${grpc_target} describe ${GRPC_SERVICE} 2>&1)
if ! echo "${grpc_methods}" | grep -q "rpc ${GRPC_METHOD}"; then
    echo -e "${RED}❌ 方法 ${GRPC_METHOD} 不存在于服务 ${GRPC_SERVICE} 中${NC}"
    echo "可用方法: ${grpc_methods}"
    exit 1
fi

# 调用gRPC方法（使用符合ListPoliciesRequest格式的参数）
temp_grpc=$(mktemp)
grpc_request='{"offset": 0, "limit": 10}'  # 符合proto定义的请求参数

grpcurl ${tls_args} -max-time ${TIMEOUT} \
    -d "${grpc_request}" \
    ${grpc_target} ${GRPC_SERVICE}/${GRPC_METHOD} > "${temp_grpc}" 2>&1

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ gRPC方法调用成功${NC}"
    echo -e "\n${YELLOW}gRPC响应内容:${NC}"
    cat "${temp_grpc}" | python3 -m json.tool  # 格式化JSON响应
else
    echo -e "${RED}❌ gRPC方法调用失败${NC}"
    echo "错误信息: $(cat ${temp_grpc})"
    exit 1
fi
rm -f "${temp_grpc}"

print_separator
echo -e "${GREEN}所有服务（HTTP/HTTPS/gRPC）验证通过${NC}"
exit 0