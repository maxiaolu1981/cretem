#!/bin/bash
set -euo pipefail

# 程序名称（根据实际可执行文件名称修改）
PROGRAM_NAME="iam-apiserver"

# 自动补全核心脚本内容
COMPLETION_SCRIPT=$(cat <<'EOF'
#!/bin/bash

_iam_apiserver_completions() {
    local cur prev words cword
    # 初始化补全环境（依赖bash-completion提供的函数）
    _init_completion || return

    # 定义子命令（根据你的实际子命令修改）
    local subcommands="start stop restart status version"
    
    # 定义所有支持的flag（根据你的NamedFlagSets实际标志修改）
    local flags="
        --help -h
        --version
        # generic组
        --generic-log-level
        --generic-metrics-bind-address
        --generic-health-probe-bind-address
        # jwt组
        --jwt-secret
        --jwt-expiration
        # grpc组
        --grpc-port
        --grpc-max-recv-message-size
        # mysql组
        --mysql-host
        --mysql-port
        --mysql-username
        --mysql-password
        --mysql-database
        # redis组
        --redis-url
        --redis-password
        --redis-db
        # features组
        --features-enable-beta
        # 服务配置组
        --insecure-serving-bind-address
        --secure-serving-bind-port
        --secure-serving-cert-file
        # 日志组
        --logs-format
        --logs-level
        --logs-output-paths
    "

    # 补全逻辑
    if [ $cword -eq 1 ]; then
        # 第一个参数：补全子命令或flag
        COMPREPLY=($(compgen -W "${subcommands} ${flags}" -- "$cur"))
        return 0
    fi

    # 补全flag（以--或-开头）
    if [[ $cur == --* || $cur == -* ]]; then
        COMPREPLY=($(compgen -W "${flags}" -- "$cur"))
        return 0
    fi

    # 子命令后的参数补全（按需扩展）
    local prev_cmd=${words[1]}
    case $prev_cmd in
        status)
            # 示例：status子命令可补全服务名
            COMPREPLY=($(compgen -W "api-server db-service" -- "$cur"))
            ;;
        *)
            # 其他情况默认补全flag
            COMPREPLY=($(compgen -W "${flags}" -- "$cur"))
            ;;
    esac
}

# 注册补全函数到目标程序
complete -F _iam_apiserver_completions iam-apiserver
EOF
)

# 开始安装流程
echo "=== 开始安装 ${PROGRAM_NAME} 自动补全 ==="

# 1. 安装必要的bash-completion（仅补全依赖）
echo "-> 安装bash-completion基础组件..."
sudo apt update -qq >/dev/null
sudo apt install -y -qq bash-completion >/dev/null

# 2. 创建补全目录并写入脚本
echo "-> 配置补全规则..."
mkdir -p ~/.bash_completion.d
echo "${COMPLETION_SCRIPT}" > ~/.bash_completion.d/${PROGRAM_NAME}.bash

# 3. 配置.bashrc实现永久生效（避免重复添加）
echo "-> 设置永久生效..."
if ! grep -q "bash_completion.d/*" ~/.bashrc; then
    cat <<'EOT' >> ~/.bashrc

# 加载自定义命令补全脚本
for bcfile in ~/.bash_completion.d/*; do
    [ -f "$bcfile" ] && . "$bcfile"
done
EOT
fi

# 4. 立即生效，无需重启终端
echo "-> 应用配置..."
source ~/.bash_completion.d/${PROGRAM_NAME}.bash
source ~/.bashrc

echo "=== 安装完成！==="
echo "测试方法：输入 '${PROGRAM_NAME} ' 后按Tab键，即可看到补全提示"
