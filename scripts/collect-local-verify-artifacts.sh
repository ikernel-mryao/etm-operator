#!/bin/bash
set -euo pipefail

# =============================================================================
# collect-local-verify-artifacts.sh
# 用途：全面收集 etmem-operator 相关的集群状态、日志和资源信息
# 用于调试、问题排查和验证部署状态
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
ARTIFACTS_DIR="${REPO_DIR}/artifacts"
mkdir -p "${ARTIFACTS_DIR}"

cd "${REPO_DIR}"

# 使用时间戳创建本次收集的子目录
TIMESTAMP=$(date '+%Y%m%d-%H%M%S')
COLLECT_DIR="${ARTIFACTS_DIR}/collect-${TIMESTAMP}"
mkdir -p "${COLLECT_DIR}"

# 收集成功计数
COLLECTED=0
FAILED=0

echo "=============================================="
echo " etmem-operator 集群信息全面收集"
echo "=============================================="
echo ""
echo "收集时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "输出目录: ${COLLECT_DIR}"
echo ""

# 安全执行命令并保存输出的辅助函数
collect() {
    local desc="$1"
    local filename="$2"
    shift 2
    local output_file="${COLLECT_DIR}/${filename}"

    echo -n "  收集: ${desc}..."
    if "$@" > "${output_file}" 2>&1; then
        echo " ✅"
        COLLECTED=$((COLLECTED + 1))
    else
        echo " ⚠️ （部分数据或命令失败）"
        FAILED=$((FAILED + 1))
    fi
}

# -----------------------------------------------
# 【步骤 1】检查集群连接
# -----------------------------------------------
echo "【步骤 1】检查集群连接..."
if ! kubectl cluster-info &>/dev/null; then
    echo "❌ 无法连接到 Kubernetes 集群，无法收集信息"
    echo "  请确保集群可用后重试"
    exit 1
fi
echo "✅ 集群连接正常"
echo ""

# -----------------------------------------------
# 【步骤 2】收集 etmem-system 命名空间资源
# -----------------------------------------------
echo "【步骤 2】收集 etmem-system 命名空间资源..."
collect "etmem-system 全部资源" "etmem-system-all.txt" \
    kubectl get all -n etmem-system -o wide

collect "etmem-system Pod 详情" "etmem-system-pods-detail.txt" \
    kubectl describe pods -n etmem-system

# -----------------------------------------------
# 【步骤 3】收集 CRD 信息
# -----------------------------------------------
echo ""
echo "【步骤 3】收集 etmem 相关 CRD 信息..."
collect "etmem CRD 列表" "etmem-crds.txt" \
    bash -c "kubectl get crd 2>&1 | grep -E 'NAME|etmem' || echo '未找到 etmem 相关 CRD'"

# -----------------------------------------------
# 【步骤 4】收集 EtmemPolicy 资源
# -----------------------------------------------
echo ""
echo "【步骤 4】收集 EtmemPolicy 资源..."
collect "EtmemPolicy 列表 (YAML)" "etmem-policies.yaml" \
    bash -c "kubectl get etmempolicy -A -o yaml 2>&1 || echo '无法获取 EtmemPolicy 或资源不存在'"

# -----------------------------------------------
# 【步骤 5】收集 EtmemNodeState 资源
# -----------------------------------------------
echo ""
echo "【步骤 5】收集 EtmemNodeState 资源..."
collect "EtmemNodeState 列表 (YAML)" "etmem-nodestates.yaml" \
    bash -c "kubectl get etmemnodestate -A -o yaml 2>&1 || echo '无法获取 EtmemNodeState 或资源不存在'"

# -----------------------------------------------
# 【步骤 6】收集测试 Pod 信息
# -----------------------------------------------
echo ""
echo "【步骤 6】收集测试 Pod 信息..."
collect "memhog-test Pod (YAML)" "memhog-test-pod.yaml" \
    bash -c "kubectl get pod memhog-test -o yaml 2>&1 || echo 'memhog-test Pod 不存在'"

# -----------------------------------------------
# 【步骤 7】收集组件日志
# -----------------------------------------------
echo ""
echo "【步骤 7】收集组件日志..."
collect "Operator 日志 (最近 200 行)" "operator-logs.txt" \
    bash -c "kubectl -n etmem-system logs deploy/etmem-operator --tail=200 2>&1 || echo '无法获取 Operator 日志'"

collect "Agent 日志 (最近 200 行)" "agent-logs.txt" \
    bash -c "kubectl -n etmem-system logs ds/etmem-agent --tail=200 2>&1 || echo '无法获取 Agent 日志'"

# -----------------------------------------------
# 【步骤 8】收集集群事件
# -----------------------------------------------
echo ""
echo "【步骤 8】收集集群事件..."
collect "全部事件（按时间排序）" "cluster-events.txt" \
    bash -c "kubectl get events -A --sort-by=.lastTimestamp 2>&1 || echo '无法获取事件'"

collect "etmem-system 命名空间事件" "etmem-system-events.txt" \
    bash -c "kubectl get events -n etmem-system --sort-by=.lastTimestamp 2>&1 || echo '无法获取 etmem-system 事件'"

# -----------------------------------------------
# 【步骤 9】收集集群基本信息
# -----------------------------------------------
echo ""
echo "【步骤 9】收集集群基本信息..."
collect "集群信息" "cluster-info.txt" \
    kubectl cluster-info

collect "节点信息" "nodes.txt" \
    kubectl get nodes -o wide

collect "命名空间列表" "namespaces.txt" \
    kubectl get namespaces

# -----------------------------------------------
# 【步骤 10】生成收集摘要
# -----------------------------------------------
echo ""
echo "【步骤 10】生成收集摘要..."

SUMMARY_FILE="${COLLECT_DIR}/00-summary.txt"
cat > "${SUMMARY_FILE}" << EOF
==============================================
 etmem-operator 信息收集摘要
==============================================

收集时间:     $(date '+%Y-%m-%d %H:%M:%S')
收集目录:     ${COLLECT_DIR}
成功收集:     ${COLLECTED} 项
收集失败:     ${FAILED} 项

--- 文件列表 ---
EOF

ls -lh "${COLLECT_DIR}/" >> "${SUMMARY_FILE}"

echo ""
echo "=============================================="
echo " 信息收集完成"
echo "=============================================="
echo ""
echo "📂 输出目录:    ${COLLECT_DIR}"
echo "✅ 成功收集:    ${COLLECTED} 项"
if [ ${FAILED} -gt 0 ]; then
    echo "⚠️  收集失败:    ${FAILED} 项"
fi
echo ""
echo "--- 收集的文件列表 ---"
ls -lh "${COLLECT_DIR}/"
echo ""
echo "摘要文件: ${SUMMARY_FILE}"
