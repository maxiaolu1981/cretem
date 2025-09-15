#!/bin/bash
echo "=== Grafana数据流测试 ==="

# 1. 检查数据在Prometheus中是否存在
echo "1. 检查Prometheus中的MySQL数据:"
curl -s "http://localhost:9090/api/v1/query?query=mysql_up" | jq '.data.result[0].value[1]'

# 2. 检查Grafana数据源连接
echo "2. 测试Grafana数据源:"
# 这需要在Grafana界面中测试

# 3. 检查最近数据点
echo "3. 最近的数据抓取:"
curl -s "http://localhost:9090/api/v1/targets" | jq -r '.data.activeTargets[] | select(.labels.job=="mysql") | .lastScrape'

# 4. 检查指标列表
echo "4. 可用的MySQL指标示例:"
curl -s "http://localhost:9090/api/v1/label/__name__/values" | jq -r '.data[]' | grep mysql | head -5

echo "=== 测试完成 ==="