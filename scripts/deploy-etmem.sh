#!/bin/bash
set -euo pipefail

# =============================================================================
# deploy-etmem.sh
# 用途：编译并部署 etmem-operator 和 etmem-agent 到 Kubernetes 集群
# 包含：前置检查、Go 编译、镜像构建、Helm 部署、状态验证
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
ARTIFACTS_DIR="${REPO_DIR}/artifacts"
mkdir -p "${ARTIFACTS_DIR}"

cd "${REPO_DIR}"

DEPLOY_STATUS_FILE="${ARTIFACTS_DIR}/deploy-status.txt"
: > "${DEPLOY_STATUS_FILE}"

# 记录部署状态的辅助函数
log_status() {
    local msg="$1"
    echo "$msg" | tee -a "${DEPLOY_STATUS_FILE}"
}

# 镜像版本
IMAGE_TAG="${IMAGE_TAG:-v0.2.0}"
OPERATOR_IMAGE="etmem-operator:${IMAGE_TAG}"
AGENT_IMAGE="etmem-agent:${IMAGE_TAG}"
HELM_RELEASE="etmem-operator"
HELM_NAMESPACE="etmem-system"

echo "=============================================="
echo " etmem-operator 编译与部署"
echo "=============================================="
echo ""
log_status "部署开始时间: $(date '+%Y-%m-%d %H:%M:%S')"
log_status ""

# -----------------------------------------------
# 【步骤 1】检查前置依赖
# -----------------------------------------------
echo "【步骤 1】检查前置依赖..."
MISSING_DEPS=()

# 检查 Go
if command -v go &>/dev/null; then
    GO_VERSION=$(go version 2>&1)
    log_status "✅ Go 已安装: ${GO_VERSION}"
else
    MISSING_DEPS+=("go (>= 1.21)")
    log_status "❌ Go 未安装"
fi

# 检查容器工具（优先使用 nerdctl）
CONTAINER_TOOL=""
if command -v nerdctl &>/dev/null; then
    CONTAINER_TOOL="nerdctl"
    log_status "✅ nerdctl 已安装（将使用 nerdctl 构建镜像）"
elif command -v docker &>/dev/null; then
    CONTAINER_TOOL="docker"
    log_status "✅ docker 已安装（将使用 docker 构建镜像）"
else
    MISSING_DEPS+=("nerdctl 或 docker")
    log_status "❌ 未找到容器构建工具（nerdctl/docker）"
fi

# 检查 Helm
if command -v helm &>/dev/null; then
    log_status "✅ Helm 已安装: $(helm version --short 2>/dev/null)"
else
    MISSING_DEPS+=("helm (>= 3)")
    log_status "❌ Helm 未安装"
fi

# 检查 kubectl
if command -v kubectl &>/dev/null; then
    log_status "✅ kubectl 已安装"
else
    MISSING_DEPS+=("kubectl")
    log_status "❌ kubectl 未安装"
fi

if [ ${#MISSING_DEPS[@]} -gt 0 ]; then
    echo ""
    echo "❌ 缺少以下依赖，无法继续部署："
    for dep in "${MISSING_DEPS[@]}"; do
        echo "   - ${dep}"
    done
    echo ""
    echo "请先安装以上工具后重试。"
    log_status "❌ 部署中止：缺少依赖"
    exit 1
fi

# -----------------------------------------------
# 【步骤 2】编译 Go 二进制文件
# -----------------------------------------------
echo ""
echo "【步骤 2】编译 Go 二进制文件..."
mkdir -p "${REPO_DIR}/bin"

echo "  编译 operator..."
CGO_ENABLED=0 GOOS=linux go build -o bin/operator ./cmd/operator
log_status "✅ operator 编译成功: bin/operator"

echo "  编译 agent..."
CGO_ENABLED=0 GOOS=linux go build -o bin/agent ./cmd/agent
log_status "✅ agent 编译成功: bin/agent"

echo "✅ 所有二进制文件编译完成"

# -----------------------------------------------
# 【步骤 3】构建容器镜像
# -----------------------------------------------
echo ""
echo "【步骤 3】构建容器镜像..."

if [ "${CONTAINER_TOOL}" = "nerdctl" ]; then
    # 使用 nerdctl 构建到 k8s.io 命名空间，以便 containerd 直接使用
    echo "  使用 nerdctl (k8s.io 命名空间) 构建 operator 镜像..."
    nerdctl --namespace k8s.io build -t "${OPERATOR_IMAGE}" -f Dockerfile.operator .
    log_status "✅ operator 镜像构建成功: ${OPERATOR_IMAGE} (nerdctl/k8s.io)"

    echo "  使用 nerdctl (k8s.io 命名空间) 构建 agent 镜像..."
    nerdctl --namespace k8s.io build -t "${AGENT_IMAGE}" -f Dockerfile.agent .
    log_status "✅ agent 镜像构建成功: ${AGENT_IMAGE} (nerdctl/k8s.io)"
else
    # 使用 docker 构建
    echo "  使用 docker 构建 operator 镜像..."
    docker build -t "${OPERATOR_IMAGE}" -f Dockerfile.operator .
    log_status "✅ operator 镜像构建成功: ${OPERATOR_IMAGE} (docker)"

    echo "  使用 docker 构建 agent 镜像..."
    docker build -t "${AGENT_IMAGE}" -f Dockerfile.agent .
    log_status "✅ agent 镜像构建成功: ${AGENT_IMAGE} (docker)"
fi

echo "✅ 所有容器镜像构建完成"

# -----------------------------------------------
# 【步骤 4】使用 Helm 部署到集群
# -----------------------------------------------
echo ""
echo "【步骤 4】使用 Helm 部署 etmem-operator..."

# 检查集群连接
if ! kubectl cluster-info &>/dev/null; then
    echo "❌ 无法连接到 Kubernetes 集群，请先确保集群可用"
    log_status "❌ 部署失败：无法连接到集群"
    exit 1
fi

helm upgrade --install "${HELM_RELEASE}" ./deploy/helm \
    -n "${HELM_NAMESPACE}" \
    --create-namespace \
    --set operator.image="${OPERATOR_IMAGE}" \
    --set agent.image="${AGENT_IMAGE}" \
    --wait --timeout 120s 2>&1 | tee -a "${DEPLOY_STATUS_FILE}"

log_status "✅ Helm 部署完成"

# -----------------------------------------------
# 【步骤 5】等待 Pod 就绪并验证状态
# -----------------------------------------------
echo ""
echo "【步骤 5】等待 Pod 就绪..."

# 等待 operator 部署就绪
echo "  等待 etmem-operator 部署就绪..."
if kubectl -n "${HELM_NAMESPACE}" rollout status deploy/etmem-operator --timeout=120s 2>&1; then
    log_status "✅ etmem-operator 部署就绪"
else
    log_status "⚠️  etmem-operator 部署等待超时"
fi

# 等待 agent DaemonSet 就绪
echo "  等待 etmem-agent DaemonSet 就绪..."
if kubectl -n "${HELM_NAMESPACE}" rollout status ds/etmem-agent --timeout=120s 2>&1; then
    log_status "✅ etmem-agent DaemonSet 就绪"
else
    log_status "⚠️  etmem-agent DaemonSet 等待超时"
fi

# 输出 Pod 状态
echo ""
echo "--- etmem-system 命名空间 Pod 状态 ---"
kubectl get pods -n "${HELM_NAMESPACE}" -o wide 2>&1 | tee -a "${DEPLOY_STATUS_FILE}"

# -----------------------------------------------
# 汇总结果
# -----------------------------------------------
echo ""
log_status ""
log_status "部署结束时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "=============================================="
echo " 部署完成"
echo "=============================================="
echo "部署状态已保存至: ${DEPLOY_STATUS_FILE}"
