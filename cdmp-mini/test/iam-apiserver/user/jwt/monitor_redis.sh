#!/bin/bash

# 颜色定义
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 简单的阈值检查函数
check_threshold() {
    local value=$1
    local warn=$2
    local danger=$3
    local desc=$4
    
    if [ $value -gt $danger ]; then
        echo -e "   ${RED}⚠️  $desc: $value (> $danger)${NC}"
    elif [ $value -gt $warn ]; then
        echo -e "   ${YELLOW}⚠️  $desc: $value (> $warn)${NC}"
    else
        echo -e "   ${GREEN}✅ $desc: $value${NC}"
    fi
}

# 主监控函数
monitor_redis() {
    echo -e "${BLUE}=== 🚀 Redis实时监控 ===${NC}"
    echo -e "时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo ""
    
    # 获取Redis信息
    local info=$(redis-cli info 2>/dev/null)
    
    # 提取指标
    local ops=$(echo "$info" | awk -F: '/instantaneous_ops_per_sec/{print $2}' | tr -d '\r')
    local clients=$(echo "$info" | awk -F: '/connected_clients/{print $2}' | tr -d '\r')
    local memory=$(echo "$info" | awk -F: '/used_memory_human/{print $2}' | tr -d '\r')
    local blocked=$(echo "$info" | awk -F: '/blocked_clients/{print $2}' | tr -d '\r')
    local cpu_sys=$(echo "$info" | awk -F: '/used_cpu_sys/{print $2}' | tr -d '\r')
    local cpu_user=$(echo "$info" | awk -F: '/used_cpu_user/{print $2}' | tr -d '\r')
    
    # 显示基本信息
    echo -e "${BLUE}📊 基本指标:${NC}"
    echo -e "   OPS: ${GREEN}$ops${NC}"
    echo -e "   连接数: ${GREEN}$clients${NC}"
    echo -e "   内存使用: ${GREEN}$memory${NC}"
    echo -e "   阻塞客户端: ${GREEN}$blocked${NC}"
    
    echo ""
    echo -e "${BLUE}⚠️  健康检查:${NC}"
    
    # 正确的阈值检查
    check_threshold $ops 8000 15000 "OPS"
    check_threshold $clients 300 800 "连接数"
    check_threshold $blocked 5 20 "阻塞客户端"
    
    # 显示CPU累计时间（不是百分比）
    echo -e "   📊 CPU累计使用时间:"
    echo -e "     系统: ${GREEN}$cpu_sys 秒${NC}"
    echo -e "     用户: ${GREEN}$cpu_user 秒${NC}"
    
    echo ""
    echo -e "${BLUE}========================================${NC}"
}

monitor_redis