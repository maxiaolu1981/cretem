 watch -n 2 'echo -n "总行数: "; wc -l 2025-10-18.json | awk "{print \$1}"; echo -n "文件大小: "; du -h 2025-10-18.json | awk "{print \$1}"'
----持续监测文件大小及行数

watch -n 2 'mysql -uroot -p'"'"'iam59!z$'"'"' -e "SELECT COUNT(*) FROM 数据库名.user"'
---持续监测数据库表记录
✅ 调试时：通过 trace-id 查看完整调用链

✅ 监控时：通过 metrics 查看性能指标

✅ 审计时：通过 audit log 满足合规要求

✅ 排错时：通过 error log 快速定位问题
grep -E "createperf_serialbtp89a_77qq.*trace|trace.*crea.createperf_serialbtp89a_77qq" /var/log/iam/iam-apiserver.log

pgo
GOCPUPROFILE=web.pprof go run apiserver.go --log.level debug
ls -lh app.pprof
go build -pgo=app.pprof -o app-optimized .

cd /home/mxl/cretem/cretem && grep -n "用户创建链路耗时超过200ms" log/iam-apiserver.log | tail

SELECT * FROM mysql.slow_log ORDER BY start_time DESC LIMIT 5;
ls -l --block-size=M

cd /home/mxl/cretem/cretem/log && python - <<'PY'
> import re, json
> from statistics import mean
> import math
> file='iam-apiserver.log'
> pattern=re.compile(r'"kafka_consumption_ms":([0-9.]+)')
> values=[]
> with open(file,'r') as f:
> for line in f:
> for match in pattern.finditer(line):
> try:
> values.append(float(match.group(1)))
> except ValueError:
> pass
> if not values:
> print('no values')
> else:
> print('count', len(values))
> print('min', min(values))
> print('max', max(values))
> avg=sum(values)/len(values)
> print('mean', avg)
> values_sorted=sorted(values)
> def pct(p):
> idx=int(math.ceil(p/100*len(values_sorted)))-1
> idx=max(0, min(idx, len(values_sorted)-1))
> return values_sorted[idx]
> for p in (50,90,95,99):
> print(f'p{p}', pct(p))
> PY

cd /home/mxl/cretem/cretem/log && python3 - <<'PY'
> import re, math
> file='iam-apiserver.log'
> pattern=re.compile(r'"api_processing_ms":([0-9.]+)')
> values=[]
> with open(file,'r') as f:
> for line in f:
> for match in pattern.finditer(line):
> try:
> values.append(float(match.group(1)))
> except ValueError:
> pass
> if not values:
> print('no values')
> else:
> print('count', len(values))
> print('min', min(values))
> print('max', max(values))
> avg=sum(values)/len(values)
> print('mean', avg)
> values_sorted=sorted(values)
> def pct(p):
> idx=int(math.ceil(p/100*len(values_sorted)))-1
> idx=max(0, min(idx, len(values_sorted)-1))
> return values_sorted[idx]
> for p in (50,90,95,99):
> print(f'p{p}', pct(p))
> PY
SET GLOBAL slow_query_log = 'ON';
SET GLOBAL long_query_time = 0.05;  -- 单位秒，按需要调整

go test -v -run TestCreatePerformance -timeout 1000m ./...
pgrep -f iam-apiserver
find . -maxdepth 2 -name 'web.pprof'
pprof -alloc_space <http://localhost:8088/debug/pprof/heap>
