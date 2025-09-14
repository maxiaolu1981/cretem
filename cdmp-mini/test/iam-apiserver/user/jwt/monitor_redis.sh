#!/bin/bash
echo "=== Redis实时监控 ==="
echo "时间: $(date)"
echo "OPS: $(redis-cli info stats | grep instantaneous_ops_per_sec | cut -d: -f2)"
echo "连接数: $(redis-cli info clients | grep connected_clients | cut -d: -f2)"
echo "内存: $(redis-cli info memory | grep used_memory_human | cut -d: -f2)"
echo "CPU系统: $(redis-cli info cpu | grep used_cpu_sys | cut -d: -f2)"
echo "CPU用户: $(redis-cli info cpu | grep used_cpu_user | cut -d: -f2)"
echo "阻塞客户端: $(redis-cli info clients | grep blocked_clients | cut -d: -f2)"