 watch -n 2 'echo -n "总行数: "; wc -l 2025-10-18.json | awk "{print \$1}"; echo -n "文件大小: "; du -h 2025-10-18.json | awk "{print \$1}"'
----持续监测文件大小及行数
