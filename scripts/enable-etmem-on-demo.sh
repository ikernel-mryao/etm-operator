#!/bin/bash
set -euo pipefail

# =============================================================================
# enable-etmem-on-demo.sh
# 用途：为测试 Pod 启用 etmem 并验证 Operator 是否正确响应
# 包含：添加标签、等待调谐、检查 EtmemPolicy、Agent 日志、NodeState
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
ARTIFACTS_DIR="${REPO_DIR}/artifacts"
mkdir -p "${ARTIFACTS_DIR}"

cd "${REPO_DIR}"

ENABLE_RESULTS_FILE="${ARTIFACTS_DIR}/enable-results.txt"
: > "${ENABLE_RESULTS_FILE}"

# 记录结果的辅助函数
log_result() {
    local msg="$1"
    echo "$msg" | tee -a "${ENABLE_RESULTS_FILE}"
}

# 参数
POD_NAME="memhog-test"
POD_NAMESPACE="${POD_NAMESPACE:-default}"
ETMEM_NAMESPACE="etmem-system"
RECONCILE_WAIT="${RECONCILE_WAIT:-35}"

echo "=============================================="
echo " 在测试 Pod 上启用 etmem"
echo "=============================================="
echo ""
log_result "启用时间: $(date '+%Y-%m-%d %H:%M:%S')"
log_result ""

# -----------------------------------------------
# 【步骤 1】检查测试 Pod 是否存在且在运行
# -----------------------------------------------
echo "【步骤 1】检查测试 Pod 是否存在..."
POD_PHASE=$(kubectl get pod "${POD_NAME}" -n "${POD_NAMESPACE}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")

if [ -z "${POD_PHASE}" ]; then
    echo "❌ 未找到 Pod ${POD_NAME}，请先运行 deploy-demo-app.sh"
    log_result "❌ 未找到 Pod ${POD_NAME}"
    exit 1
fi

if [ "${POD_PHASE}" != "Running" ]; then
    echo "⚠️  Pod ${POD_NAME} 当前状态为 ${POD_PHASE}，不是 Running"
    log_result "⚠️  Pod 状态异常: ${POD_PHASE}"
fi

log_result "✅ Pod ${POD_NAME} 存在，状态: ${POD_PHASE}"

# -----------------------------------------------
# 【步骤 2】添加 etmem 启用标签
# -----------------------------------------------
echo ""
echo "【步骤 2】为 Pod 添加 etmem 启用标签..."
kubectl label pod "${POD_NAME}" -n "${POD_NAMESPACE}" \
    etmem.openeuler.io/enable=true --overwrite 2>&1 | tee -a "${ENABLE_RESULTS_FILE}"
log_result "✅ 已添加标签: etmem.openeuler.io/enable=true"

# 确认标签已设置
echo "  当前 Pod 标签:"
kubectl get pod "${POD_NAME}" -n "${POD_NAMESPACE}" --show-labels 2>&1 | tee -a "${ENABLE_RESULTS_FILE}"

# -----------------------------------------------
# 【步骤 3】等待 Operator 调谐
# -----------------------------------------------
echo ""
echo "【步骤 3】等待 Operator 完成调谐（${RECONCILE_WAIT} 秒）..."
echo "  调谐过程中 Operator 将创建 EtmemPolicy 资源并通知 Agent..."

# 显示倒计时
for i in $(seq "${RECONCILE_WAIT}" -5 1); do
    echo -ne "  剩余等待时间: ${i} 秒...\r"
    sleep 5
done
echo "  等待完成                          "
log_result ""
log_result "✅ 调谐等待完成（${RECONCILE_WAIT}s）"

# -----------------------------------------------
# 【步骤 4】检查 EtmemPolicy 是否已创建
# -----------------------------------------------
echo ""
echo "【步骤 4】检查 EtmemPolicy 资源..."
log_result ""
log_result "--- EtmemPolicy 列表 ---"

if kubectl get etmempolicy -A 2>&1 | tee -a "${ENABLE_RESULTS_FILE}" | grep -q "etmem"; then
    log_result "✅ 检测到 EtmemPolicy 资源"
else
    log_result "⚠️  未检测到 EtmemPolicy 资源（可能 Operator 尚未完成调谐或 CRD 未就绪）"
fi

# -----------------------------------------------
# 【步骤 5】检查 Agent 日志中的匹配信息
# -----------------------------------------------
echo ""
echo "【步骤 5】检查 Agent 日志中的 Pod 匹配信息..."
log_result ""
log_result "--- Agent 日志（匹配相关） ---"

AGENT_LOGS=$(kubectl -n "${ETMEM_NAMESPACE}" logs ds/etmem-agent --tail=100 2>&1 || echo "无法获取 Agent 日志")
echo "${AGENT_LOGS}" | grep -i "matched\|match.*pod\|${POD_NAME}" 2>/dev/null | tee -a "${ENABLE_RESULTS_FILE}" || true

if echo "${AGENT_LOGS}" | grep -qi "matched.*pod\|match.*${POD_NAME}"; then
    log_result "✅ Agent 已匹配到目标 Pod"
else
    log_result "⚠️  Agent 日志中未找到明确的 Pod 匹配记录"
fi

# -----------------------------------------------
# 【步骤 6】检查 EtmemNodeState
# -----------------------------------------------
echo ""
echo "【步骤 6】检查 EtmemNodeState 资源..."
log_result ""
log_result "--- EtmemNodeState 列表 ---"

if kubectl get etmemnodestate -A 2>&1 | tee -a "${ENABLE_RESULTS_FILE}" | grep -q "etmem"; then
    log_result "✅ 检测到 EtmemNodeState 资源"
else
    log_result "⚠️  未检测到 EtmemNodeState 资源"
fi

# -----------------------------------------------
# 【步骤 7】设置 profile 注解（可选）
# -----------------------------------------------
echo ""
echo "【步骤 7】设置 etmem profile 注解（aggressive 模式）..."
kubectl annotate pod "${POD_NAME}" -n "${POD_NAMESPACE}" \
    etmem.openeuler.io/profile=aggressive --overwrite 2>&1 | tee -a "${ENABLE_RESULTS_FILE}"
log_result "✅ 已设置注解: etmem.openeuler.io/profile=aggressive"

# -----------------------------------------------
# 汇总
# -----------------------------------------------
echo ""
log_result ""
log_result "启用验证结束时间: $(date '+%Y-%m-%d %H:%M:%S')"

echo "=============================================="
echo " etmem 启用验证完成"
echo "=============================================="
echo ""
echo "Pod 名称:       ${POD_NAME}"
echo "etmem 标签:     etmem.openeuler.io/enable=true"
echo "profile 注解:   etmem.openeuler.io/profile=aggressive"
echo ""
echo "详细结果已保存至: ${ENABLE_RESULTS_FILE}"
