#!/bin/bash

# 大并发系统参数检查脚本
# 适用于 Ubuntu 22.04 Server

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 阈值定义
THRESHOLDS=(
    "file_max=1000000"           # 文件描述符阈值
    "tcp_tw_reuse=1"             # TCP TIME_WAIT 重用
    "tcp_max_syn_backlog=2048"   # SYN backlog 大小
    "somaxconn=2048"             # 最大连接队列
    "netdev_max_backlog=2000"    # 网络设备 backlog
    "rmem_max=134217728"         # 读缓冲区大小
    "wmem_max=134217728"         # 写缓冲区大小
    "tcp_rmem=4096 65536 134217728"  # TCP 读内存
    "tcp_wmem=4096 65536 134217728"  # TCP 写内存
    "tcp_mem=786432 1048576 1572864" # TCP 内存
    "ip_local_port_range=1024 65535" # 本地端口范围
)

print_header() {
    echo -e "${BLUE}==============================================${NC}"
    echo -e "${BLUE}   Ubuntu 22.04 大并发系统参数检查${NC}"
    echo -e "${BLUE}==============================================${NC}"
    echo ""
}

print_section() {
    echo -e "${YELLOW}[$1]${NC} $2"
    echo "----------------------------------------------"
}

check_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓ $2${NC}"
    else
        echo -e "${RED}✗ $2${NC}"
    fi
}

# 1. 检查文件描述符限制
check_file_descriptors() {
    print_section "1" "文件描述符限制检查"
    
    local current_soft=$(ulimit -Sn)
    local current_hard=$(ulimit -Hn)
    local sys_max=$(cat /proc/sys/fs/file-max)
    
    echo "当前软限制: $current_soft"
    echo "当前硬限制: $current_hard" 
    echo "系统最大文件数: $sys_max"
    
    if [ $current_soft -lt 100000 ]; then
        echo -e "${RED}警告: 文件描述符软限制过低 (< 100000)${NC}"
    else
        echo -e "${GREEN}正常: 文件描述符限制足够${NC}"
    fi
    
    if [ $sys_max -lt 1000000 ]; then
        echo -e "${RED}警告: 系统文件数限制过低 (< 1000000)${NC}"
    fi
}

# 2. 检查网络参数
check_network_params() {
    print_section "2" "网络参数检查"
    
    # TCP 参数
    local tcp_tw_reuse=$(cat /proc/sys/net/ipv4/tcp_tw_reuse)
    local tcp_max_syn_backlog=$(cat /proc/sys/net/ipv4/tcp_max_syn_backlog)
    local somaxconn=$(cat /proc/sys/net/core/somaxconn)
    local netdev_backlog=$(cat /proc/sys/net/core/netdev_max_backlog)
    
    echo "TCP TIME_WAIT 重用: $tcp_tw_reuse (推荐: 1)"
    echo "SYN Backlog 大小: $tcp_max_syn_backlog (推荐: ≥2048)"
    echo "最大连接队列: $somaxconn (推荐: ≥2048)"
    echo "网络设备 Backlog: $netdev_backlog (推荐: ≥2000)"
    
    [ $tcp_tw_reuse -eq 1 ] || echo -e "${YELLOW}建议: 启用 tcp_tw_reuse${NC}"
    [ $tcp_max_syn_backlog -ge 2048 ] || echo -e "${YELLOW}建议: 增加 tcp_max_syn_backlog${NC}"
    [ $somaxconn -ge 2048 ] || echo -e "${YELLOW}建议: 增加 somaxconn${NC}"
}

# 3. 检查内存参数
check_memory_params() {
    print_section "3" "内存和缓冲区检查"
    
    local rmem_max=$(cat /proc/sys/net/core/rmem_max)
    local wmem_max=$(cat /proc/sys/net/core/wmem_max)
    local tcp_rmem=$(cat /proc/sys/net/ipv4/tcp_rmem)
    local tcp_wmem=$(cat /proc/sys/net/ipv4/tcp_wmem)
    local tcp_mem=$(cat /proc/sys/net/ipv4/tcp_mem)
    
    echo "最大读缓冲区: $rmem_max (推荐: ≥134217728)"
    echo "最大写缓冲区: $wmem_max (推荐: ≥134217728)"
    echo "TCP 读内存: $tcp_rmem"
    echo "TCP 写内存: $tcp_wmem"
    echo "TCP 内存: $tcp_mem"
    
    [ $rmem_max -ge 134217728 ] || echo -e "${YELLOW}建议: 增加 rmem_max${NC}"
    [ $wmem_max -ge 134217728 ] || echo -e "${YELLOW}建议: 增加 wmem_max${NC}"
}

# 4. 检查端口范围
check_port_range() {
    print_section "4" "端口范围检查"
    
    local port_range=$(cat /proc/sys/net/ipv4/ip_local_port_range)
    local start_port=$(echo $port_range | cut -d' ' -f1)
    local end_port=$(echo $port_range | cut -d' ' -f2)
    local total_ports=$((end_port - start_port + 1))
    
    echo "本地端口范围: $port_range"
    echo "可用端口数: $total_ports"
    
    if [ $total_ports -lt 60000 ]; then
        echo -e "${RED}警告: 可用端口数不足 (< 60000)${NC}"
    else
        echo -e "${GREEN}正常: 端口范围足够${NC}"
    fi
}

# 5. 检查系统资源使用
check_system_resources() {
    print_section "5" "系统资源使用情况"
    
    # 内存使用
    local mem_total=$(free -m | awk 'NR==2{print $2}')
    local mem_used=$(free -m | awk 'NR==2{print $3}')
    local mem_usage=$((mem_used * 100 / mem_total))
    
    # CPU 负载
    local load_avg=$(cat /proc/loadavg | cut -d' ' -f1-3)
    local cpu_cores=$(nproc)
    
    # 网络连接数
    local tcp_connections=$(ss -s | awk '/^TCP:/{print $2}')
    local established_conn=$(ss -s | awk '/estab/{print $2}')
    
    echo "内存使用: $mem_used/$mem_total MB ($mem_usage%)"
    echo "CPU 负载: $load_avg (核心数: $cpu_cores)"
    echo "TCP 连接数: $tcp_connections"
    echo "已建立连接: $established_conn"
    
    if [ $mem_usage -gt 80 ]; then
        echo -e "${RED}警告: 内存使用率过高${NC}"
    fi
    
    local load1=$(echo $load_avg | cut -d' ' -f1)
    if [ $(echo "$load1 > $cpu_cores" | bc) -eq 1 ]; then
        echo -e "${RED}警告: 系统负载过高${NC}"
    fi
}

# 6. 检查当前进程限制
check_process_limits() {
    print_section "6" "进程限制检查"
    
    echo "当前进程ID: $$"
    echo "检查当前进程的限制..."
    
    # 检查当前shell的限制
    ulimit -a | grep -E "(open files|max user processes)"
    
    # 检查系统级限制
    local pid_max=$(cat /proc/sys/kernel/pid_max)
    local threads_max=$(cat /proc/sys/kernel/threads-max)
    
    echo "系统最大PID: $pid_max"
    echo "系统最大线程数: $threads_max"
}

# 7. 生成优化建议
generate_recommendations() {
    print_section "7" "优化建议"
    
    echo -e "${GREEN}立即优化建议:${NC}"
    echo "1. 临时调整文件描述符限制:"
    echo "   ulimit -n 100000"
    echo ""
    echo "2. 临时调整网络参数:"
    echo "   echo 1 > /proc/sys/net/ipv4/tcp_tw_reuse"
    echo "   echo 2048 > /proc/sys/net/core/somaxconn"
    echo ""
    echo -e "${YELLOW}永久优化建议 (编辑 /etc/sysctl.conf):${NC}"
    cat << EOF
# 大并发优化配置
net.core.somaxconn = 2048
net.core.netdev_max_backlog = 2000
net.ipv4.tcp_max_syn_backlog = 2048
net.ipv4.tcp_tw_reuse = 1
net.ipv4.ip_local_port_range = 1024 65535
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 65536 134217728
net.ipv4.tcp_wmem = 4096 65536 134217728
fs.file-max = 1000000
EOF

    echo ""
    echo -e "${YELLOW}编辑 limits.conf:${NC}"
    cat << EOF
# /etc/security/limits.conf
* soft nofile 100000
* hard nofile 100000
* soft nproc 65536
* hard nproc 65536
EOF
}

# 8. 快速优化函数
quick_optimize() {
    read -p "是否立即应用临时优化? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "应用临时优化..."
        
        # 文件描述符
        ulimit -n 100000 2>/dev/null || echo "需要root权限调整文件限制"
        
        # 网络参数 (需要root)
        if [ $EUID -eq 0 ]; then
            echo 1 > /proc/sys/net/ipv4/tcp_tw_reuse
            echo 2048 > /proc/sys/net/core/somaxconn
            echo 2048 > /proc/sys/net/ipv4/tcp_max_syn_backlog
            echo "临时优化完成"
        else
            echo "需要root权限来调整系统参数"
            echo "请使用: sudo $0"
        fi
    fi
}

# 主函数
main() {
    print_header
    
    check_file_descriptors
    check_network_params
    check_memory_params
    check_port_range
    check_system_resources
    check_process_limits
    generate_recommendations
    
    echo ""
    quick_optimize
    
    echo -e "${GREEN}检查完成!${NC}"
}

# 执行主函数
main "$@"