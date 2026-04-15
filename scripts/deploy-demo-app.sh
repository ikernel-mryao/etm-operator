#!/bin/bash
set -euo pipefail

# =============================================================================
# deploy-demo-app.sh
# 用途：部署测试工作负载 memhog-test Pod（不启用 etmem 标签）
# 该 Pod 模拟内存密集型工作负载，用于后续验证 etmem 功能
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
ARTIFACTS_DIR="${REPO_DIR}/artifacts"
mkdir -p "${ARTIFACTS_DIR}"

cd "${REPO_DIR}"

DEMO_APP_FILE="${ARTIFACTS_DIR}/demo-app.txt"
: > "${DEMO_APP_FILE}"

# 记录信息的辅助函数
log_info() {
    local msg="$1"
    echo "$msg" | tee -a "${DEMO_APP_FILE}"
}

# 测试 Pod 名称
POD_NAME="memhog-test"
POD_NAMESPACE="${POD_NAMESPACE:-default}"

echo "=============================================="
echo " 部署测试工作负载 (${POD_NAME})"
echo "=============================================="
echo ""
log_info "部署时间: $(date '+%Y-%m-%d %H:%M:%S')"
log_info ""

# -----------------------------------------------
# 【步骤 1】检查集群连接
# -----------------------------------------------
echo "【步骤 1】检查集群连接..."
if ! kubectl cluster-info &>/dev/null; then
    echo "❌ 无法连接到 Kubernetes 集群"
    echo "请先运行 setup-local-cluster.sh 配置集群"
    log_info "❌ 部署失败：无法连接到集群"
    exit 1
fi
log_info "✅ 集群连接正常"

# -----------------------------------------------
# 【步骤 2】检查是否已存在同名 Pod
# -----------------------------------------------
echo ""
echo "【步骤 2】检查是否已存在 ${POD_NAME} Pod..."
if kubectl get pod "${POD_NAME}" -n "${POD_NAMESPACE}" &>/dev/null; then
    echo "⚠️  ${POD_NAME} 已存在，将先删除旧 Pod..."
    kubectl delete pod "${POD_NAME}" -n "${POD_NAMESPACE}" --wait=true --timeout=60s
    log_info "⚠️  已删除旧的 ${POD_NAME} Pod"
    echo "  等待旧 Pod 完全删除..."
    sleep 5
fi

# -----------------------------------------------
# 【步骤 3】创建并应用 Pod 清单
# -----------------------------------------------
echo ""
echo "【步骤 3】创建测试 Pod（不启用 etmem 标签）..."

# 注意：此处故意不添加 etmem.openeuler.io/enable=true 标签
# 后续通过 enable-etmem-on-demo.sh 脚本手动启用
cat <<'EOF' | kubectl apply -n "${POD_NAMESPACE}" -f -
apiVersion: v1
kind: Pod
metadata:
  name: memhog-test
  labels:
    app: memhog-test
spec:
  containers:
  - name: memhog
    image: busybox:latest
    command: ["sh", "-c", "while true; do dd if=/dev/zero of=/dev/null bs=1M count=100; sleep 10; done"]
    resources:
      requests:
        memory: "128Mi"
      limits:
        memory: "256Mi"
  restartPolicy: Always
EOF

log_info "✅ Pod 清单已应用（命名空间: ${POD_NAMESPACE}）"
log_info "   注意：未添加 etmem.openeuler.io/enable=true 标签"

# -----------------------------------------------
# 【步骤 4】等待 Pod 运行就绪
# -----------------------------------------------
echo ""
echo "【步骤 4】等待 ${POD_NAME} 运行就绪..."

MAX_WAIT=120
INTERVAL=5
ELAPSED=0

while [ ${ELAPSED} -lt ${MAX_WAIT} ]; do
    POD_PHASE=$(kubectl get pod "${POD_NAME}" -n "${POD_NAMESPACE}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "未知")

    if [ "${POD_PHASE}" = "Running" ]; then
        echo "✅ ${POD_NAME} 已进入 Running 状态"
        log_info "✅ Pod 状态: Running"
        break
    fi

    echo "  当前状态: ${POD_PHASE}，等待中... (${ELAPSED}/${MAX_WAIT}s)"
    sleep ${INTERVAL}
    ELAPSED=$((ELAPSED + INTERVAL))
done

if [ ${ELAPSED} -ge ${MAX_WAIT} ]; then
    echo "⚠️  等待 Pod 就绪超时（${MAX_WAIT}s），当前状态: ${POD_PHASE}"
    log_info "⚠️  Pod 等待超时，当前状态: ${POD_PHASE}"
fi

# -----------------------------------------------
# 【步骤 5】输出 Pod 详细信息
# -----------------------------------------------
echo ""
echo "【步骤 5】输出 Pod 详细信息..."
log_info ""
log_info "--- Pod 状态 ---"
kubectl get pod "${POD_NAME}" -n "${POD_NAMESPACE}" -o wide 2>&1 | tee -a "${DEMO_APP_FILE}"

log_info ""
log_info "--- Pod 标签 ---"
kubectl get pod "${POD_NAME}" -n "${POD_NAMESPACE}" --show-labels 2>&1 | tee -a "${DEMO_APP_FILE}"

log_info ""
log_info "--- Pod 描述（事件部分） ---"
kubectl describe pod "${POD_NAME}" -n "${POD_NAMESPACE}" 2>&1 | tail -20 | tee -a "${DEMO_APP_FILE}"

# -----------------------------------------------
# 汇总
# -----------------------------------------------
echo ""
echo "=============================================="
echo " 测试工作负载部署完成"
echo "=============================================="
echo ""
echo "Pod 名称:    ${POD_NAME}"
echo "命名空间:    ${POD_NAMESPACE}"
echo "etmem 标签:  未设置（使用 enable-etmem-on-demo.sh 启用）"
echo ""
echo "详细信息已保存至: ${DEMO_APP_FILE}"
