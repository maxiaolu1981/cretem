#!/bin/bash
# analyze-api-performance.sh - 修复版：正确提取Prometheus指标

# ========================== 基础配置 ==========================
API_HOST="localhost"
API_PORT="8080"
HEALTH_CHECK_ENDPOINT="/healthz"
LOGIN_ENDPOINT="/login"
V1_USERS_ENDPOINT="/v1/users"
METRICS_ENDPOINT="/metrics"
PPROF_ROOT_ENDPOINT="/debug/pprof/"
PPROF_CPU_ENDPOINT="/debug/pprof/profile?seconds=10"
PPROF_HEAP_ENDPOINT="/debug/pprof/heap"
ADMIN_USER="admin"
ADMIN_PASS="Admin@2021"
REQUEST_ID_HEADER="X-Request-Id"
LOAD_TEST_DURATION=20
LOAD_TEST_CONCURRENCY=5
TIMEOUT=15
LOAD_TEST_LOG="/tmp/load_test_log_$(date +%s).txt"

# ========================== 颜色定义 ==========================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# ========================== 工具函数 ==========================
print_separator() {
    echo -e "\n${BLUE}==================================================${NC}"
}

print_title() {
    echo -e "\n${CYAN}===== $1 =====${NC}"
}

print_load_test_summary() {
    echo -e "\n${YELLOW}   负载测试请求摘要：${NC}"
    if [ -f "${LOAD_TEST_LOG}" ]; then
        total=$(wc -l < "${LOAD_TEST_LOG}")
        success=$(grep "200\|202" "${LOAD_TEST_LOG}" | wc -l)
        error=$(grep -v "200\|202" "${LOAD_TEST_LOG}" | wc -l)
        echo -e "   总请求数：${total}"
        echo -e "   成功请求（200/202）：${success}"
        echo -e "   错误请求：${error}"
    else
        echo -e "   日志文件不存在，无法统计"
    fi
}

http_request() {
    local api_url=$1
    local need_token=$2
    local method=${3:-GET}
    local data=${4:-""}
    local log_response=${5:-false}

    local temp_body="/tmp/api_body_$(date +%s%N)"
    local temp_headers="/tmp/api_headers_$(date +%s%N)"

    curl_cmd=(
        curl -s --no-buffer
        -w "%{http_code}"
        -o "${temp_body}"
        -D "${temp_headers}"
        -X "${method}"
        -H "${REQUEST_ID_HEADER}: ${GLOBAL_REQUEST_ID}"
        --max-time ${TIMEOUT}
    )

    [ "${need_token}" = "true" ] && [ -n "${TOKEN}" ] && curl_cmd+=(-H "Authorization: Bearer ${TOKEN}")
    [ "${method}" = "POST" ] && curl_cmd+=(-H "Content-Type: application/json")
    [ -n "${data}" ] && curl_cmd+=(-d "${data}")
    curl_cmd+=("${api_url}")

    status_code=$("${curl_cmd[@]}")
    body=$(cat "${temp_body}" 2>/dev/null)
    headers=$(cat "${temp_headers}" 2>/dev/null)

    if [ "${log_response}" = "true" ]; then
        echo "${status_code} ${api_url}" >> "${LOAD_TEST_LOG}" 2>/dev/null
    fi

    sleep 0.1
    rm -f "${temp_body}" "${temp_headers}"

    echo "${status_code} ${body} ${headers}"
}

check_dependencies() {
    print_title "检查依赖工具"
    local dependencies=("curl" "python3" "go" "bc" "xmllint")
    for dep in "${dependencies[@]}"; do
        if ! command -v "${dep}" &> /dev/null; then
            echo -e "${RED}❌ 缺少必要工具：${dep}${NC}"
            exit 1
        fi
    done
    echo -e "${GREEN}✅ 所有依赖工具已安装${NC}"
}

# ========================== 会话初始化 ==========================
init_session() {
    print_title "初始化会话"
    
    health_url="http://${API_HOST}:${API_PORT}${HEALTH_CHECK_ENDPOINT}"
    echo -e "${YELLOW}🔍 验证健康检查接口：${health_url}${NC}"
    read health_status health_body _ <<< $(http_request "${health_url}" "false")
    
    if [ "${health_status}" -ne 200 ]; then
        echo -e "${RED}❌ 健康检查接口未响应（状态码：${health_status}）${NC}"
        exit 1
    fi
    echo -e "${GREEN}✅ 健康检查接口正常${NC}"

    login_url="http://${API_HOST}:${API_PORT}${LOGIN_ENDPOINT}"
    login_data=$(printf '{"username":"%s","password":"%s"}' "${ADMIN_USER}" "${ADMIN_PASS}")
    echo -e "${YELLOW}🔍 请求登录接口：${login_url}${NC}"
    
    local login_temp_body="/tmp/login_body_$(date +%s%N)"
    local login_temp_headers="/tmp/login_headers_$(date +%s%N)"
    login_status=$(curl -s --no-buffer \
        -w "%{http_code}" \
        -o "${login_temp_body}" \
        -D "${login_temp_headers}" \
        -X POST \
        -H "Content-Type: application/json" \
        --max-time ${TIMEOUT} \
        -d "${login_data}" \
        "${login_url}")
    login_body=$(cat "${login_temp_body}")
    login_headers=$(cat "${login_temp_headers}")
    rm -f "${login_temp_body}" "${login_temp_headers}"

    TOKEN=$(echo "${login_body}" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    GLOBAL_REQUEST_ID=$(echo "${login_headers}" | grep -i "^${REQUEST_ID_HEADER}:" | awk -F ': ' '{print $2}' | tr -d '\r')

    if [ -n "${TOKEN}" ]; then
        echo -e "${YELLOW}🔍 验证Token有效性（访问${V1_USERS_ENDPOINT}）${NC}"
        test_url="http://${API_HOST}:${API_PORT}${V1_USERS_ENDPOINT}"
        read test_status _ _ <<< $(http_request "${test_url}" "true")
        echo -e "   验证请求状态码：${test_status}"
        if [ "${test_status}" -eq 401 ] || [ "${test_status}" -eq 403 ]; then
            echo -e "${RED}❌ Token无效或权限不足${NC}"
            exit 1
        fi
    else
        echo -e "${RED}❌ 未获取到Token${NC}"
        exit 1
    fi

    echo -e "${GREEN}✅ 会话初始化成功（Token有效）${NC}"
}

# ========================== 验证性能功能 ==========================
validate_features() {
    print_title "验证性能分析功能"
    
    metrics_url="http://${API_HOST}:${API_PORT}${METRICS_ENDPOINT}"
    echo -e "${YELLOW}🔍 验证Prometheus接口：${metrics_url}${NC}"
    local metrics_temp="/tmp/metrics_data_$(date +%s%N)"
    metrics_status=$(curl -s --no-buffer -w "%{http_code}" -o "${metrics_temp}" --max-time ${TIMEOUT} "${metrics_url}")
    metrics_body=$(cat "${metrics_temp}")
    rm -f "${metrics_temp}"

    if [ "${metrics_status}" -ne 200 ]; then
        echo -e "${RED}❌ Prometheus接口不可用${NC}"
        exit 1
    fi
    echo -e "${GREEN}✅ Prometheus监控功能正常${NC}"

    echo -e "\n${YELLOW}🔍 验证pprof功能（根接口：${PPROF_ROOT_ENDPOINT}）${NC}"
    pprof_url="http://${API_HOST}:${API_PORT}${PPROF_ROOT_ENDPOINT}"
    local pprof_temp="/tmp/pprof_root_$(date +%s%N)"
    pprof_status=$(curl -s --no-buffer -w "%{http_code}" -o "${pprof_temp}" --max-time ${TIMEOUT} "${pprof_url}")
    pprof_body=$(cat "${pprof_temp}")
    rm -f "${pprof_temp}"

    pprof_text=$(echo "${pprof_body}" | xmllint --html --xpath "string(//body)" - 2>/dev/null | tr -d '\n' | sed 's/  */ /g')
    if [ "${pprof_status}" -eq 200 ] && echo "${pprof_text}" | grep -qi "Types of profiles available"; then
        echo -e "${GREEN}✅ pprof性能分析功能正常${NC}"
    else
        echo -e "${RED}❌ pprof功能未生效${NC}"
        exit 1
    fi
}

# ========================== 负载测试 ==========================
run_load_test() {
    print_title "执行负载测试"
    users_url="http://${API_HOST}:${API_PORT}${V1_USERS_ENDPOINT}"
    echo -e "   测试接口：${users_url}"
    echo -e "   配置：持续${LOAD_TEST_DURATION}秒，并发${LOAD_TEST_CONCURRENCY}请求"
    echo -e "   日志文件：${LOAD_TEST_LOG}"

    > "${LOAD_TEST_LOG}" 2>/dev/null

    for i in $(seq 1 ${LOAD_TEST_CONCURRENCY}); do
        (
            end_time=$((SECONDS + LOAD_TEST_DURATION))
            while [ ${SECONDS} -lt ${end_time} ]; do
                http_request "${users_url}" "true" "GET" "" "true"
                sleep 0.1
            done
        ) &
    done

    echo -e "${YELLOW}   压测进行中...${NC}"
    for ((i=LOAD_TEST_DURATION; i>0; i--)); do
        echo -ne "   剩余时间：${i}秒\r"
        sleep 1
    done
    wait
    echo -e "\n${GREEN}✅ 负载测试完成${NC}"
    print_load_test_summary
}

# ========================== 修复：正确提取Prometheus指标 ==========================
analyze_prometheus() {
    print_title "Prometheus性能指标分析（/v1/users接口）"
    metrics_url="http://${API_HOST}:${API_PORT}${METRICS_ENDPOINT}"
    local metrics_temp="/tmp/metrics_analyze_$(date +%s%N)"
    curl -s --no-buffer -o "${metrics_temp}" --max-time ${TIMEOUT} "${metrics_url}"
    metrics_body=$(cat "${metrics_temp}")
    rm -f "${metrics_temp}"

    # 1. 显示原始指标用于调试
    echo -e "${YELLOW}   📊 Prometheus中/v1/users的原始指标：${NC}"
    local users_raw_metrics=$(echo "${metrics_body}" | grep "url=\"/v1/users\"" | head -5)
    if [ -z "${users_raw_metrics}" ]; then
        echo -e "     （未找到/v1/users的指标）"
        echo -e "${YELLOW}   调试信息：尝试查找所有包含/v1/users的指标...${NC}"
        echo "${metrics_body}" | grep -i "v1/users" | head -5 | sed 's/^/     /'
    else
        echo "${users_raw_metrics}" | sed 's/^/     /'
    fi

    # 2. 修复：正确提取请求总数（所有状态码）
    local total_users_requests=0
    local success_users_requests=0
    
    # 提取所有/v1/users接口的请求指标
    local users_metrics=$(echo "${metrics_body}" | grep "url=\"/v1/users\"" | grep "gin_requests_total")
    
    if [ -n "${users_metrics}" ]; then
        # 计算总请求数（所有状态码）
        total_users_requests=$(echo "${users_metrics}" | awk -F' ' '{print $2}' | tr -d '\r' | awk '{sum += $1} END {print sum}')
        
        # 计算成功请求数（状态码200）
        success_metrics=$(echo "${users_metrics}" | grep "code=\"200\"")
        if [ -n "${success_metrics}" ]; then
            success_users_requests=$(echo "${success_metrics}" | awk -F' ' '{print $2}' | tr -d '\r' | awk '{sum += $1} END {print sum}')
        fi
    fi

    # 3. 提取延迟指标
    local p50_latency=$(echo "${metrics_body}" | \
        grep "url=\"/v1/users\"" | grep "quantile=\"0.5\"" | \
        awk -F' ' '{printf "%.2fms", $2*1000}' | head -1)

    local p90_latency=$(echo "${metrics_body}" | \
        grep "url=\"/v1/users\"" | grep "quantile=\"0.9\"" | \
        awk -F' ' '{printf "%.2fms", $2*1000}' | head -1)

    # 4. 计算错误率
    local error_rate="0.00"
    if [ "${total_users_requests}" -gt 0 ]; then
        local error_count=$((total_users_requests - success_users_requests))
        error_rate=$(echo "scale=2; (${error_count}/${total_users_requests})*100" | bc)
    fi

    # 5. 输出指标结果
    echo -e "\n${CYAN}===== /v1/users接口核心性能指标 ====="
    echo -e "   总请求数（Prometheus统计）：${total_users_requests}"
    echo -e "   成功请求数（200）：${success_users_requests}"
    
    if [ -f "${LOAD_TEST_LOG}" ]; then
        load_test_total=$(wc -l < "${LOAD_TEST_LOG}")
        echo -e "   负载测试请求数：${load_test_total}"
        
        # 计算差异
        local diff=$((total_users_requests - load_test_total))
        if [ ${diff} -ne 0 ]; then
            echo -e "   请求数差异：${diff}（包含历史请求）"
        fi
    fi
    
    echo -e "   平均吞吐量：$(echo "scale=2; ${total_users_requests}/${LOAD_TEST_DURATION}" | bc) 请求/秒"
    echo -e "   响应延迟 (P50)：${p50_latency:-N/A}"
    echo -e "   响应延迟 (P90)：${p90_latency:-N/A}"
    echo -e "   错误率：${error_rate}%${NC}"

    # 6. 诊断结果
    echo -e "\n${YELLOW}   🔍 诊断结果：${NC}"
    if [ "${total_users_requests}" -gt 0 ]; then
        echo -e "   ${GREEN}✅ 指标提取成功！Prometheus统计到 ${total_users_requests} 次请求${NC}"
        
        if [ "${total_users_requests}" -lt "${load_test_total}" ]; then
            echo -e "   ${YELLOW}⚠️  提示：Prometheus统计的请求数少于负载测试，可能指标尚未完全刷新${NC}"
        fi
    else
        echo -e "   ${RED}❌ 未提取到指标，请检查：${NC}"
        echo -e "     1. 执行: curl ${metrics_url} | grep '/v1/users'"
        echo -e "     2. 确认应用正确配置了Prometheus指标导出"
        echo -e "     3. 检查指标名称前缀是否为 'gin_requests_total'"
    fi
}

# ========================== pprof深度分析 ==========================
analyze_pprof() {
    print_title "pprof性能深度分析"
    local temp_dir="/tmp/pprof_temp_$(date +%s%N)"
    mkdir -p "${temp_dir}"

    echo -e "${YELLOW}   采集CPU使用数据（10秒）...${NC}"
    cpu_profile="${temp_dir}/cpu.pprof"
    curl -s --no-buffer -o "${cpu_profile}" \
        -H "${REQUEST_ID_HEADER}: ${GLOBAL_REQUEST_ID}" \
        "http://${API_HOST}:${API_PORT}${PPROF_CPU_ENDPOINT}"

    echo -e "${YELLOW}   采集内存分配数据...${NC}"
    mem_profile="${temp_dir}/mem.pprof"
    curl -s --no-buffer -o "${mem_profile}" \
        -H "${REQUEST_ID_HEADER}: ${GLOBAL_REQUEST_ID}" \
        "http://${API_HOST}:${API_PORT}${PPROF_HEAP_ENDPOINT}"

    if [ ! -s "${mem_profile}" ]; then
        echo -e "${RED}❌ 内存数据采集失败${NC}"
        rm -rf "${temp_dir}"
        return
    fi

    print_title "内存分配热点（Top 10）"
    echo -e "${CYAN}   函数名 | 分配内存 | 占比${NC}"
    go tool pprof -top -sample_index=alloc_space "${mem_profile}" 2>/dev/null | \
        grep -A10 "^Showing" | tail -n +2 | head -10 | \
        awk '{printf "   %-40s %8s %6s\n", substr($0, index($0,$6)), $1, $3}'

    if [ -s "${cpu_profile}" ]; then
        print_title "CPU热点函数（Top 10）"
        echo -e "${CYAN}   函数名 | 采样时间 | 占比${NC}"
        go tool pprof -top -sample_index=cpu "${cpu_profile}" 2>/dev/null | \
            grep -A10 "^Showing" | tail -n +2 | head -10 | \
            awk '{printf "   %-40s %8s %6s\n", substr($0, index($0,$6)), $1, $3}'
    fi

    rm -rf "${temp_dir}"
}

# ========================== 主流程 ==========================
main() {
    trap 'rm -f "${LOAD_TEST_LOG}" 2>/dev/null' EXIT

    echo -e "${CYAN}===== API性能分析工具（修复版） =====${NC}"
    check_dependencies
    init_session
    validate_features
    run_load_test
    analyze_prometheus
    analyze_pprof

    print_separator
    echo -e "${GREEN}🎉 所有性能分析流程完成！${NC}"
    print_separator
}

main