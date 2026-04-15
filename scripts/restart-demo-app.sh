#!/bin/bash
set -euo pipefail

# =============================================================================
# restart-demo-app.sh
# 用途：测试 etmem-operator 和 etmem-agent 的重启恢复能力
# 分别重启 Operator 和 Agent，验证策略和状态是否正确恢复
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
ARTIFACTS_DIR="${REPO_DIR}/artifacts"
mkdir -p "${ARTIFACTS_DIR}"

cd "${REPO_DIR}"

RESTART_RESULTS_FILE="${ARTIFACTS_DIR}/restart-results.txt"
: > "${RESTART_RESULTS_FILE}"

# 记录结果的辅助函数
log_result() {
    local msg="$1"
    echo "$msg" | tee -a "${RESTART_RESULTS_FILE}"
}

# 参数
ETMEM_NAMESPACE="etmem-system"
OPERATOR_WAIT="${OPERATOR_WAIT:-45}"
AGENT_WAIT="${AGENT_WAIT:-45}"

echo "=============================================="
echo " etmem 组件重启恢复测试"
echo "=============================================="
echo ""
log_result "测试开始时间: $(date '+%Y-%m-%d %H:%M:%S')"
log_result ""

# -----------------------------------------------
# 【步骤 1】记录重启前的状态快照
# -----------------------------------------------
echo "【步骤 1】记录重启前的状态快照..."
log_result "=== 重启前状态快照 ==="
log_result ""

log_result "--- EtmemPolicy 列表 ---"
kubectl get etmempolicy -A 2>&1 | tee -a "${RESTART_RESULTS_FILE}" || true

log_result ""
log_result "--- EtmemNodeState 列表 ---"
kubectl get etmemnodestate -A 2>&1 | tee -a "${RESTART_RESULTS_FILE}" || true

log_result ""
log_result "--- etmem-system Pod 状态 ---"
kubectl get pods -n "${ETMEM_NAMESPACE}" -o wide 2>&1 | tee -a "${RESTART_RESULTS_FILE}" || true

# 保存策略数量用于后续比较
POLICY_COUNT_BEFORE=$(kubectl get etmempolicy -A --no-headers 2>/dev/null | wc -l || echo "0")
log_result ""
log_result "重启前 EtmemPolicy 数量: ${POLICY_COUNT_BEFORE}"

echo "✅ 状态快照已记录"

# -----------------------------------------------
# 【步骤 2】重启 Operator
# -----------------------------------------------
echo ""
echo "【步骤 2】重启 etmem-operator..."
log_result ""
log_result "=== 重启 Operator ==="

kubectl -n "${ETMEM_NAMESPACE}" rollout restart deploy/etmem-operator 2>&1 | tee -a "${RESTART_RESULTS_FILE}"
log_result "✅ Operator 重启命令已发送"

echo "  等待 Operator 重启完成（${OPERATOR_WAIT} 秒）..."
sleep "${OPERATOR_WAIT}"

# -----------------------------------------------
# 【步骤 3】验证 Operator 恢复状态
# -----------------------------------------------
echo ""
echo "【步骤 3】验证 Operator 恢复状态..."
log_result ""
log_result "--- Operator 重启后状态 ---"

# 检查 Operator Pod 是否正常运行
OPERATOR_STATUS=$(kubectl -n "${ETMEM_NAMESPACE}" get pods -l app=etmem-operator -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "未知")
log_result "Operator Pod 状态: ${OPERATOR_STATUS}"

if [ "${OPERATOR_STATUS}" = "Running" ]; then
    log_result "✅ Operator 重启后正常运行"
else
    log_result "❌ Operator 重启后状态异常: ${OPERATOR_STATUS}"
fi

# 检查 EtmemPolicy 是否仍然存在
POLICY_COUNT_AFTER_OPERATOR=$(kubectl get etmempolicy -A --no-headers 2>/dev/null | wc -l || echo "0")
log_result "Operator 重启后 EtmemPolicy 数量: ${POLICY_COUNT_AFTER_OPERATOR}"

if [ "${POLICY_COUNT_AFTER_OPERATOR}" -ge "${POLICY_COUNT_BEFORE}" ] 2>/dev/null; then
    log_result "✅ EtmemPolicy 资源在 Operator 重启后仍然存在"
else
    log_result "⚠️  EtmemPolicy 数量变化（之前: ${POLICY_COUNT_BEFORE}, 之后: ${POLICY_COUNT_AFTER_OPERATOR}）"
fi

# 检查 Operator 日志
log_result ""
log_result "--- Operator 重启后日志（最近 30 行） ---"
kubectl -n "${ETMEM_NAMESPACE}" logs deploy/etmem-operator --tail=30 2>&1 | tee -a "${RESTART_RESULTS_FILE}" || true

# -----------------------------------------------
# 【步骤 4】重启 Agent（通过删除 Pod 触发 DaemonSet 重建）
# -----------------------------------------------
echo ""
echo "【步骤 4】重启 etmem-agent（删除 Pod 触发 DaemonSet 重建）..."
log_result ""
log_result "=== 重启 Agent ==="

kubectl -n "${ETMEM_NAMESPACE}" delete pod -l app=etmem-agent 2>&1 | tee -a "${RESTART_RESULTS_FILE}"
log_result "✅ Agent Pod 删除命令已发送，等待 DaemonSet 重新创建..."

echo "  等待 Agent 重启完成（${AGENT_WAIT} 秒）..."
sleep "${AGENT_WAIT}"

# -----------------------------------------------
# 【步骤 5】验证 Agent 恢复状态
# -----------------------------------------------
echo ""
echo "【步骤 5】验证 Agent 恢复状态..."
log_result ""
log_result "--- Agent 重启后状态 ---"

# 检查 Agent Pod 是否正常运行
AGENT_PODS=$(kubectl -n "${ETMEM_NAMESPACE}" get pods -l app=etmem-agent --no-headers 2>/dev/null || echo "")
if echo "${AGENT_PODS}" | grep -q "Running"; then
    log_result "✅ Agent 重启后正常运行"
    echo "${AGENT_PODS}" | tee -a "${RESTART_RESULTS_FILE}"
else
    log_result "❌ Agent 重启后状态异常"
    echo "${AGENT_PODS}" | tee -a "${RESTART_RESULTS_FILE}"
fi

# 检查 Agent 是否重新匹配了目标 Pod
log_result ""
log_result "--- Agent 重启后日志（Pod 匹配相关） ---"
AGENT_LOGS=$(kubectl -n "${ETMEM_NAMESPACE}" logs ds/etmem-agent --tail=50 2>&1 || echo "无法获取 Agent 日志")
echo "${AGENT_LOGS}" | grep -i "matched\|match.*pod\|reconcil\|memhog" 2>/dev/null | tee -a "${RESTART_RESULTS_FILE}" || true

if echo "${AGENT_LOGS}" | grep -qi "matched\|match.*pod\|memhog"; then
    log_result "✅ Agent 重启后重新匹配到目标 Pod"
else
    log_result "⚠️  Agent 日志中未找到明确的重新匹配记录"
fi

# -----------------------------------------------
# 【步骤 6】最终状态汇总
# -----------------------------------------------
echo ""
echo "【步骤 6】最终状态汇总..."
log_result ""
log_result "=== 最终状态汇总 ==="
log_result ""

log_result "--- etmem-system Pod 最终状态 ---"
kubectl get pods -n "${ETMEM_NAMESPACE}" -o wide 2>&1 | tee -a "${RESTART_RESULTS_FILE}" || true

log_result ""
log_result "--- EtmemPolicy 最终列表 ---"
kubectl get etmempolicy -A 2>&1 | tee -a "${RESTART_RESULTS_FILE}" || true

log_result ""
log_result "--- EtmemNodeState 最终列表 ---"
kubectl get etmemnodestate -A 2>&1 | tee -a "${RESTART_RESULTS_FILE}" || true

POLICY_COUNT_FINAL=$(kubectl get etmempolicy -A --no-headers 2>/dev/null | wc -l || echo "0")

log_result ""
log_result "测试结束时间: $(date '+%Y-%m-%d %H:%M:%S')"
log_result ""
log_result "--- 恢复测试总结 ---"
log_result "重启前 Policy 数量:     ${POLICY_COUNT_BEFORE}"
log_result "Operator 重启后数量:    ${POLICY_COUNT_AFTER_OPERATOR}"
log_result "Agent 重启后最终数量:   ${POLICY_COUNT_FINAL}"

echo ""
echo "=============================================="
echo " 重启恢复测试完成"
echo "=============================================="
echo "测试结果已保存至: ${RESTART_RESULTS_FILE}"
