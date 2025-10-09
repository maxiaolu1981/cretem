#!/bin/bash

# 默认参数
KEY_PATTERN=""
SHOW_VALUE=false

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -k|--key)
            KEY_PATTERN="$2"
            shift 2
            ;;
        -v|--value)
            SHOW_VALUE=true
            shift
            ;;
        -h|--help)
            echo "用法: $0 [选项]"
            echo "选项:"
            echo "  -k, --key PATTERN    指定要查找的key模式"
            echo "  -v, --value          显示匹配key的值"
            echo "  -h, --help           显示帮助信息"
            exit 0
            ;;
        *)
            echo "未知参数: $1"
            exit 1
            ;;
    esac
done

echo "=== Redis 集群状态检查 ==="

# 统计 genericapiserver:user 相关的key
echo "=== genericapiserver:user Key 统计 ==="
TOTAL_USER_KEYS=0
for port in 6379 6380 6381; do
    USER_KEY_COUNT=$(redis-cli -h 192.168.10.14 -p $port --scan --pattern "*genericapiserver:user*" | wc -l)
    echo "端口 $port: $USER_KEY_COUNT 个 genericapiserver:user key"
    TOTAL_USER_KEYS=$((TOTAL_USER_KEYS + USER_KEY_COUNT))
done
echo "总计: $TOTAL_USER_KEYS 个 genericapiserver:user key"
echo ""

# 统计 value 为 RATE_LIMIT_PREVENTION 的key数量
echo "=== RATE_LIMIT_PREVENTION Value 统计 ==="
TOTAL_RATE_LIMIT_KEYS=0
for port in 6379 6380 6381; do
    RATE_LIMIT_COUNT=0
    # 扫描所有key并检查value
    redis-cli -h 192.168.10.14 -p $port --scan --pattern "*" | while read key; do
        value=$(redis-cli -h 192.168.10.14 -p $port GET "$key" 2>/dev/null)
        if [ "$value" = "RATE_LIMIT_PREVENTION" ]; then
            RATE_LIMIT_COUNT=$((RATE_LIMIT_COUNT + 1))
        fi
    done
    echo "端口 $port: $RATE_LIMIT_COUNT 个 value=RATE_LIMIT_PREVENTION 的key"
    TOTAL_RATE_LIMIT_KEYS=$((TOTAL_RATE_LIMIT_KEYS + RATE_LIMIT_COUNT))
done
echo "总计: $TOTAL_RATE_LIMIT_KEYS 个 value=RATE_LIMIT_PREVENTION 的key"
echo ""

# 如果指定了key模式，搜索并显示
if [ -n "$KEY_PATTERN" ]; then
    echo "=== 搜索 Key 模式: $KEY_PATTERN ==="
    for port in 6379 6380 6381; do
        echo "端口 $port 的匹配结果:"
        redis-cli -h 192.168.10.14 -p $port --scan --pattern "*$KEY_PATTERN*" | while read key; do
            if [ "$SHOW_VALUE" = true ]; then
                value=$(redis-cli -h 192.168.10.14 -p $port GET "$key" 2>/dev/null)
                echo "  $key = $value"
            else
                echo "  $key"
            fi
        done
    done
    echo ""
fi

# 显示基本集群信息
echo "=== 集群基本信息 ==="
for port in 6379 6380 6381; do
    echo "端口 $port:"
    echo " - Key数量: $(redis-cli -h 192.168.10.14 -p $port DBSIZE)"
    echo " - 内存使用: $(redis-cli -h 192.168.10.14 -p $port INFO memory | grep used_memory_human | cut -d: -f2)"
    echo " - 连接地址: $(redis-cli -h 192.168.10.14 -p $port CLUSTER NODES | grep myself | awk '{print $2}')"
    echo "---"
done