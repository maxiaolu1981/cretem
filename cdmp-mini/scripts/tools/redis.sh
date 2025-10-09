#!/bin/bash

# 你的 Redis 集群节点
NODES=("192.168.10.14:6379" "192.168.10.14:6380" "192.168.10.14:6381")

echo "开始扫描 Redis 集群所有 key..."

for node in "${NODES[@]}"; do
    host=${node%:*}
    port=${node#*:}
    echo "=== 扫描节点 $host:$port ==="
    
    # 检查是否是主节点
    node_info=$(redis-cli -h $host -p $port CLUSTER NODES | grep "$host:$port")
    if echo "$node_info" | grep -q "master"; then
        echo "主节点，扫描 key..."
        redis-cli -h $host -p $port --scan --pattern "*" | while read key; do
            echo "Key: $key"
        done
    else
        echo "从节点，跳过..."
    fi
    echo
done