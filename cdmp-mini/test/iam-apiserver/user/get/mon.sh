# 实时监控缓存状态
watch -n 2 '
echo "=== 缓存命中率实时监控 ==="
curl -s "http://localhost:9090/api/v1/query?query=user_cache_hits_total" | \
  jq -r ".data.result[] | \"\(.metric.type): \(.value[1])\"" | \
  sort
echo "=========================="
'

# 计算实时命中率
curl -s "http://localhost:9090/api/v1/query?query=(
  user_cache_hits_total{type=\"hit\"} 
  + user_cache_hits_total{type=\"null_hit\"}
) 
/ 
sum(user_cache_hits_total) * 100" | \
jq -r '.data.result[0].value[1]' | \
xargs printf "当前命中率: %.2f%%\n"