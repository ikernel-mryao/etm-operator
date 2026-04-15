#!/bin/bash
set -euo pipefail

# =============================================================================
# setup-local-cluster.sh
# 用途：检查 Kubernetes 集群是否可用，若不可用则尝试创建本地集群
# 同时检查 etmem 相关的宿主机环境（etmemd、内核模块、swap）
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
ARTIFACTS_DIR="${REPO_DIR}/artifacts"
mkdir -p "${ARTIFACTS_DIR}"

ENV_CHECK_FILE="${ARTIFACTS_DIR}/env-check.txt"
: > "${ENV_CHECK_FILE}"

# 记录环境检查结果的辅助函数
log_result() {
    local msg="$1"
    echo "$msg" | tee -a "${ENV_CHECK_FILE}"
}

echo "=============================================="
echo " etmem-operator 本地集群环境检查与配置"
echo "=============================================="
echo ""

# -----------------------------------------------
# 【步骤 1】检查 kubectl 是否可用并能连接到集群
# -----------------------------------------------
echo "【步骤 1】检查 Kubernetes 集群连接状态..."
CLUSTER_READY=false

if command -v kubectl &>/dev/null; then
    log_result "✅ kubectl 已安装: $(kubectl version --client --short 2>/dev/null || kubectl version --client 2>/dev/null | head -1)"

    if kubectl cluster-info &>/dev/null; then
        CLUSTER_READY=true
        log_result "✅ 已连接到 Kubernetes 集群"
        log_result ""
        log_result "--- 集群信息 ---"
        kubectl cluster-info 2>&1 | tee -a "${ENV_CHECK_FILE}"
        log_result ""
        log_result "--- 节点列表 ---"
        kubectl get nodes -o wide 2>&1 | tee -a "${ENV_CHECK_FILE}"
        log_result ""
        echo "✅ 检测到可用的 Kubernetes 集群，跳过本地集群创建"
    else
        log_result "❌ kubectl 已安装但无法连接到集群"
    fi
else
    log_result "❌ kubectl 未安装"
fi

# -----------------------------------------------
# 【步骤 2】若无可用集群，尝试创建本地集群
# -----------------------------------------------
if [ "${CLUSTER_READY}" = false ]; then
    echo ""
    echo "【步骤 2】未检测到可用集群，尝试创建本地集群..."

    LOCAL_CLUSTER_CREATED=false

    # 尝试 kind
    if command -v kind &>/dev/null; then
        echo "  检测到 kind，尝试使用 kind 创建集群..."
        log_result "📦 使用 kind 创建本地集群..."
        if kind create cluster --name etmem-test --wait 120s 2>&1 | tee -a "${ENV_CHECK_FILE}"; then
            LOCAL_CLUSTER_CREATED=true
            log_result "✅ kind 集群创建成功 (etmem-test)"
        else
            log_result "❌ kind 集群创建失败"
        fi
    fi

    # 尝试 k3d
    if [ "${LOCAL_CLUSTER_CREATED}" = false ] && command -v k3d &>/dev/null; then
        echo "  检测到 k3d，尝试使用 k3d 创建集群..."
        log_result "📦 使用 k3d 创建本地集群..."
        if k3d cluster create etmem-test --wait 2>&1 | tee -a "${ENV_CHECK_FILE}"; then
            LOCAL_CLUSTER_CREATED=true
            log_result "✅ k3d 集群创建成功 (etmem-test)"
        else
            log_result "❌ k3d 集群创建失败"
        fi
    fi

    # 尝试 minikube
    if [ "${LOCAL_CLUSTER_CREATED}" = false ] && command -v minikube &>/dev/null; then
        echo "  检测到 minikube，尝试使用 minikube 创建集群..."
        log_result "📦 使用 minikube 创建本地集群..."
        if minikube start --profile etmem-test --wait=all 2>&1 | tee -a "${ENV_CHECK_FILE}"; then
            LOCAL_CLUSTER_CREATED=true
            log_result "✅ minikube 集群创建成功 (etmem-test)"
        else
            log_result "❌ minikube 集群创建失败"
        fi
    fi

    # 所有工具都不可用
    if [ "${LOCAL_CLUSTER_CREATED}" = false ]; then
        echo ""
        echo "❌ 无法创建本地 Kubernetes 集群！"
        echo ""
        echo "请安装以下工具之一来创建本地集群："
        echo ""
        echo "  1. kind (推荐):"
        echo "     go install sigs.k8s.io/kind@latest"
        echo "     https://kind.sigs.k8s.io/docs/user/quick-start/"
        echo ""
        echo "  2. k3d:"
        echo "     curl -s https://raw.githubusercontent.com/k3s-io/k3d/main/install.sh | bash"
        echo "     https://k3d.io/"
        echo ""
        echo "  3. minikube:"
        echo "     curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64"
        echo "     sudo install minikube-linux-amd64 /usr/local/bin/minikube"
        echo "     https://minikube.sigs.k8s.io/docs/start/"
        echo ""
        log_result "❌ 未找到可用的集群创建工具（kind/k3d/minikube）"
    fi
fi

# -----------------------------------------------
# 【步骤 3】检查 etmemd 守护进程是否在宿主机上运行
# -----------------------------------------------
echo ""
echo "【步骤 3】检查 etmemd 守护进程..."
if pgrep -x etmemd &>/dev/null; then
    log_result "✅ etmemd 守护进程正在运行 (PID: $(pgrep -x etmemd))"
else
    log_result "⚠️  etmemd 守护进程未运行（etmem 功能需要 etmemd 在宿主机上运行）"
fi

# -----------------------------------------------
# 【步骤 4】检查 etmem 相关内核模块
# -----------------------------------------------
echo ""
echo "【步骤 4】检查 etmem 相关内核模块..."
ETMEM_MODULES=("etmem" "etmem_scan" "etmem_swap")
MODULE_FOUND=false
for mod in "${ETMEM_MODULES[@]}"; do
    if lsmod 2>/dev/null | grep -qw "$mod"; then
        log_result "✅ 内核模块 ${mod} 已加载"
        MODULE_FOUND=true
    else
        log_result "⚠️  内核模块 ${mod} 未加载"
    fi
done

if [ "${MODULE_FOUND}" = false ]; then
    log_result "⚠️  未检测到任何 etmem 内核模块（可能需要 openEuler 内核支持）"
fi

# -----------------------------------------------
# 【步骤 5】检查 swap 是否启用
# -----------------------------------------------
echo ""
echo "【步骤 5】检查 swap 配置..."
SWAP_TOTAL=$(free -m 2>/dev/null | awk '/^Swap:/{print $2}' || echo "0")
if [ "${SWAP_TOTAL}" -gt 0 ] 2>/dev/null; then
    log_result "✅ Swap 已启用（总量: ${SWAP_TOTAL} MB）"
    free -m 2>/dev/null | grep Swap | tee -a "${ENV_CHECK_FILE}"
else
    log_result "⚠️  Swap 未启用或总量为 0（etmem 内存分级可能需要 swap 支持）"
fi

# -----------------------------------------------
# 汇总结果
# -----------------------------------------------
echo ""
echo "=============================================="
echo " 环境检查完成"
echo "=============================================="
echo "检查结果已保存至: ${ENV_CHECK_FILE}"
echo ""
cat "${ENV_CHECK_FILE}"
