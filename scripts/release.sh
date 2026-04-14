#!/bin/bash
# release.sh — 构建 etmem-operator 正式交付物
# 产出：Helm tgz + Operator 镜像 tar + Agent 镜像 tar
#
# 用法：./scripts/release.sh [VERSION]
# 示例：./scripts/release.sh v0.2.0
set -euo pipefail

VERSION="${1:-v0.2.0}"
OPERATOR_IMG="etmem-operator:${VERSION}"
AGENT_IMG="etmem-agent:${VERSION}"
DIST_DIR="dist/${VERSION}"

echo "=== etmem-operator release ${VERSION} ==="

mkdir -p "${DIST_DIR}"

# 1. Build Go binaries
echo "[1/5] Building binaries..."
export PATH="/usr/local/go/bin:$PATH"
make build

# 2. Build container images
echo "[2/5] Building container images..."
CONTAINER_CMD="docker"
if command -v nerdctl &>/dev/null; then
    CONTAINER_CMD="nerdctl"
fi

${CONTAINER_CMD} build -t "${OPERATOR_IMG}" -f build/operator/Dockerfile .
${CONTAINER_CMD} build -t "${AGENT_IMG}" -f build/agent/Dockerfile .

# 3. Export image tar packages
echo "[3/5] Exporting image tar packages..."
${CONTAINER_CMD} save -o "${DIST_DIR}/etmem-operator-${VERSION}.tar" "${OPERATOR_IMG}"
${CONTAINER_CMD} save -o "${DIST_DIR}/etmem-agent-${VERSION}.tar" "${AGENT_IMG}"

# 4. Package Helm chart
echo "[4/5] Packaging Helm chart..."
helm package deploy/helm/ --version "${VERSION#v}" --app-version "${VERSION#v}" -d "${DIST_DIR}/"

# 5. Summary
echo "[5/5] Release artifacts:"
ls -lh "${DIST_DIR}/"
echo ""
echo "=== Release ${VERSION} complete ==="
echo ""
echo "To install in a new environment:"
echo "  1. Load images:  ${CONTAINER_CMD} load -i ${DIST_DIR}/etmem-operator-${VERSION}.tar"
echo "  2. Load images:  ${CONTAINER_CMD} load -i ${DIST_DIR}/etmem-agent-${VERSION}.tar"
echo "  3. Install:      helm install etmem-operator ${DIST_DIR}/etmem-operator-*.tgz \\"
echo "                     --namespace etmem-system --create-namespace"
