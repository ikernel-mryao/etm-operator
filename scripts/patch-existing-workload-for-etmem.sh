#!/bin/bash
set -euo pipefail

# =============================================================================
# patch-existing-workload-for-etmem.sh
# 用途：给现有工作负载添加或移除 etmem 标签的自动化脚本
# 用法：
#   bash scripts/patch-existing-workload-for-etmem.sh <类型> <名称> [命名空间]
#   bash scripts/patch-existing-workload-for-etmem.sh <类型> <名称> [命名空间] --remove
# 示例：
#   bash scripts/patch-existing-workload-for-etmem.sh ds dbservice
#   bash scripts/patch-existing-workload-for-etmem.sh deploy myapp my-namespace
#   bash scripts/patch-existing-workload-for-etmem.sh ds dbservice default --remove
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
ARTIFACTS_DIR="${REPO_DIR}/artifacts"
mkdir -p "${ARTIFACTS_DIR}"

RESULT_FILE="${ARTIFACTS_DIR}/patch-workload-result.txt"
: > "${RESULT_FILE}"

log_info() {
    local msg="$1"
    echo "$msg" | tee -a "${RESULT_FILE}"
}

# -----------------------------------------------
# 参数解析
# -----------------------------------------------
WORKLOAD_TYPE="${1:-}"
WORKLOAD_NAME="${2:-}"
NAMESPACE="${3:-default}"
REMOVE_MODE=""

# 检查是否有 --remove 标志（可在任意位置）
for arg in "$@"; do
    if [ "$arg" = "--remove" ]; then
        REMOVE_MODE="1"
    fi
done

# 如果第三个参数是 --remove，则命名空间使用默认值
if [ "${3:-}" = "--remove" ]; then
    NAMESPACE="default"
fi

if [ -z "$WORKLOAD_TYPE" ] || [ -z "$WORKLOAD_NAME" ]; then
    echo "用法: bash $0 <类型> <名称> [命名空间] [--remove]"
    echo ""
    echo "参数说明："
    echo "  类型        工作负载类型：ds（DaemonSet）、deploy（Deployment）、sts（StatefulSet）"
    echo "  名称        工作负载名称"
    echo "  命名空间    Kubernetes 命名空间（默认: default）"
    echo "  --remove    移除 etmem 标签（而非添加）"
    echo ""
    echo "示例："
    echo "  bash $0 ds dbservice                        # 给 default 命名空间的 dbservice DaemonSet 添加标签"
    echo "  bash $0 deploy myapp my-namespace            # 给 my-namespace 的 myapp Deployment 添加标签"
    echo "  bash $0 ds dbservice default --remove        # 移除标签"
    exit 1
fi

# 验证工作负载类型
case "$WORKLOAD_TYPE" in
    ds|daemonset)
        RESOURCE_TYPE="ds"
        RESOURCE_DISPLAY="DaemonSet"
        ;;
    deploy|deployment)
        RESOURCE_TYPE="deploy"
        RESOURCE_DISPLAY="Deployment"
        ;;
    sts|statefulset)
        RESOURCE_TYPE="sts"
        RESOURCE_DISPLAY="StatefulSet"
        ;;
    *)
        echo "❌ 不支持的工作负载类型: $WORKLOAD_TYPE"
        echo "   支持的类型: ds（DaemonSet）、deploy（Deployment）、sts（StatefulSet）"
        exit 1
        ;;
esac

if [ -n "$REMOVE_MODE" ]; then
    ACTION="移除"
    OP="remove"
else
    ACTION="添加"
    OP="add"
fi

echo "=============================================="
echo " ${ACTION} etmem 标签"
echo "=============================================="
echo ""
log_info "操作时间: $(date '+%Y-%m-%d %H:%M:%S')"
log_info "操作类型: ${ACTION} etmem 标签"
log_info "工作负载: ${RESOURCE_DISPLAY} ${WORKLOAD_NAME}（命名空间: ${NAMESPACE}）"
log_info ""

# -----------------------------------------------
# 【步骤 1】检查集群连接
# -----------------------------------------------
echo "【步骤 1】检查集群连接..."
if ! kubectl cluster-info &>/dev/null; then
    echo "❌ 无法连接到 Kubernetes 集群"
    log_info "❌ 操作失败：无法连接到集群"
    exit 1
fi
log_info "✅ 集群连接正常"

# -----------------------------------------------
# 【步骤 2】验证工作负载是否存在
# -----------------------------------------------
echo ""
echo "【步骤 2】验证 ${RESOURCE_DISPLAY} ${WORKLOAD_NAME} 是否存在..."
if ! kubectl get "${RESOURCE_TYPE}" "${WORKLOAD_NAME}" -n "${NAMESPACE}" &>/dev/null; then
    echo "❌ 未找到 ${RESOURCE_DISPLAY} ${WORKLOAD_NAME}（命名空间: ${NAMESPACE}）"
    echo "   请检查名称和命名空间是否正确"
    echo ""
    echo "   可用的 ${RESOURCE_DISPLAY}："
    kubectl get "${RESOURCE_TYPE}" -n "${NAMESPACE}" --no-headers 2>/dev/null | sed 's/^/     /' || echo "     （无）"
    log_info "❌ 操作失败：工作负载不存在"
    exit 1
fi
log_info "✅ 工作负载存在"

# -----------------------------------------------
# 【步骤 3】显示当前 Pod 模板标签
# -----------------------------------------------
echo ""
echo "【步骤 3】当前 Pod 模板标签..."
log_info ""
log_info "--- 当前 Pod 模板标签 ---"
CURRENT_LABELS=$(kubectl get "${RESOURCE_TYPE}" "${WORKLOAD_NAME}" -n "${NAMESPACE}" \
    -o jsonpath='{.spec.template.metadata.labels}' 2>/dev/null || echo "{}")
echo "  ${CURRENT_LABELS}" | tee -a "${RESULT_FILE}"
echo ""

# 检查标签当前状态
HAS_LABEL=""
if echo "$CURRENT_LABELS" | grep -q "etmem.openeuler.io/enable"; then
    HAS_LABEL="1"
fi

if [ -n "$REMOVE_MODE" ] && [ -z "$HAS_LABEL" ]; then
    echo "⚠️  该工作负载当前未设置 etmem 标签，无需移除"
    log_info "⚠️  无需操作：标签不存在"
    exit 0
fi

if [ -z "$REMOVE_MODE" ] && [ -n "$HAS_LABEL" ]; then
    echo "⚠️  该工作负载已设置 etmem 标签，将覆盖更新"
    log_info "⚠️  标签已存在，将覆盖"
fi

# -----------------------------------------------
# 【步骤 4】应用 Patch
# -----------------------------------------------
echo ""
echo "【步骤 4】${ACTION} etmem 标签..."

if [ "$OP" = "add" ]; then
    PATCH='[{"op":"add","path":"/spec/template/metadata/labels/etmem.openeuler.io~1enable","value":"true"}]'
else
    PATCH='[{"op":"remove","path":"/spec/template/metadata/labels/etmem.openeuler.io~1enable"}]'
fi

echo "  执行: kubectl patch ${RESOURCE_TYPE} ${WORKLOAD_NAME} -n ${NAMESPACE} --type='json' -p='${PATCH}'"
if kubectl patch "${RESOURCE_TYPE}" "${WORKLOAD_NAME}" -n "${NAMESPACE}" \
    --type='json' -p="${PATCH}" 2>&1 | tee -a "${RESULT_FILE}"; then
    log_info "✅ Patch 成功"
else
    echo "❌ Patch 失败"
    log_info "❌ Patch 失败"
    exit 1
fi

# -----------------------------------------------
# 【步骤 5】等待滚动更新完成
# -----------------------------------------------
echo ""
echo "【步骤 5】等待滚动更新完成..."
echo "  ⚠️  Patch 会触发滚动更新，Pod 将被重建"

ROLLOUT_TIMEOUT=300

case "$RESOURCE_TYPE" in
    ds)
        echo "  等待 DaemonSet 滚动更新（超时: ${ROLLOUT_TIMEOUT}s）..."
        if kubectl rollout status "${RESOURCE_TYPE}/${WORKLOAD_NAME}" -n "${NAMESPACE}" \
            --timeout="${ROLLOUT_TIMEOUT}s" 2>&1 | tail -3 | tee -a "${RESULT_FILE}"; then
            log_info "✅ 滚动更新完成"
        else
            echo "⚠️  滚动更新超时或失败，请手动检查"
            log_info "⚠️  滚动更新超时"
        fi
        ;;
    deploy)
        echo "  等待 Deployment 滚动更新（超时: ${ROLLOUT_TIMEOUT}s）..."
        if kubectl rollout status "${RESOURCE_TYPE}/${WORKLOAD_NAME}" -n "${NAMESPACE}" \
            --timeout="${ROLLOUT_TIMEOUT}s" 2>&1 | tail -3 | tee -a "${RESULT_FILE}"; then
            log_info "✅ 滚动更新完成"
        else
            echo "⚠️  滚动更新超时或失败，请手动检查"
            log_info "⚠️  滚动更新超时"
        fi
        ;;
    sts)
        echo "  等待 StatefulSet 滚动更新（超时: ${ROLLOUT_TIMEOUT}s）..."
        if kubectl rollout status "${RESOURCE_TYPE}/${WORKLOAD_NAME}" -n "${NAMESPACE}" \
            --timeout="${ROLLOUT_TIMEOUT}s" 2>&1 | tail -3 | tee -a "${RESULT_FILE}"; then
            log_info "✅ 滚动更新完成"
        else
            echo "⚠️  滚动更新超时或失败，请手动检查"
            log_info "⚠️  滚动更新超时"
        fi
        ;;
esac

# -----------------------------------------------
# 【步骤 6】显示新 Pod 状态
# -----------------------------------------------
echo ""
echo "【步骤 6】新 Pod 状态..."
log_info ""
log_info "--- 新 Pod 状态 ---"

# 获取该工作负载管理的 Pod
SELECTOR=$(kubectl get "${RESOURCE_TYPE}" "${WORKLOAD_NAME}" -n "${NAMESPACE}" \
    -o jsonpath='{.spec.selector.matchLabels}' 2>/dev/null | \
    sed 's/[{}"]//g' | tr ',' '\n' | sed 's/:/=/g' | tr '\n' ',' | sed 's/,$//')

if [ -n "$SELECTOR" ]; then
    kubectl get pods -n "${NAMESPACE}" -l "${SELECTOR}" -o wide 2>&1 | tee -a "${RESULT_FILE}"
    echo ""
    echo "  Pod 标签："
    kubectl get pods -n "${NAMESPACE}" -l "${SELECTOR}" --show-labels 2>&1 | tee -a "${RESULT_FILE}"
else
    echo "  无法自动查找 Pod，请手动检查："
    echo "  kubectl get pods -n ${NAMESPACE} --show-labels | grep ${WORKLOAD_NAME}"
fi

# -----------------------------------------------
# 汇总
# -----------------------------------------------
echo ""
echo "=============================================="
echo " ${ACTION} etmem 标签完成"
echo "=============================================="
echo ""
echo "工作负载:     ${RESOURCE_DISPLAY} ${WORKLOAD_NAME}"
echo "命名空间:     ${NAMESPACE}"
echo "操作:         ${ACTION} etmem.openeuler.io/enable 标签"
echo ""

if [ -z "$REMOVE_MODE" ]; then
    echo "后续步骤："
    echo "  1. 等待 30-60 秒让 Agent 完成 reconcile"
    echo "  2. 检查 Agent 日志: kubectl logs -n etmem-system -l app=etmem-agent --tail=50"
    echo "  3. 检查 NodeState: kubectl get etmemnodestate -o yaml"
    echo "  4. 验证数据面: sudo bash scripts/verify-existing-workload-etmem.sh <pod-name>"
    echo "  5. 恢复: bash $0 ${WORKLOAD_TYPE} ${WORKLOAD_NAME} ${NAMESPACE} --remove"
else
    echo "工作负载已恢复到无 etmem 管理状态。"
fi
echo ""
echo "详细结果已保存至: ${RESULT_FILE}"
