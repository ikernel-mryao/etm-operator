#!/bin/bash
set -euo pipefail

# =============================================================================
# cleanup-local-env.sh
# 用途：清理 etmem-operator 测试环境中的所有资源
# 包含：测试 Pod、Helm Release、命名空间、CRD（可选）
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
ARTIFACTS_DIR="${REPO_DIR}/artifacts"
mkdir -p "${ARTIFACTS_DIR}"

cd "${REPO_DIR}"

# 是否强制删除 CRD（通过 --force 参数控制）
FORCE_DELETE_CRDS=false
for arg in "$@"; do
    if [ "$arg" = "--force" ]; then
        FORCE_DELETE_CRDS=true
    fi
done

# 清理计数
CLEANED=0
SKIPPED=0

echo "=============================================="
echo " etmem-operator 环境清理"
echo "=============================================="
echo ""
echo "清理开始时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# -----------------------------------------------
# 【步骤 1】删除测试 Pod
# -----------------------------------------------
echo "【步骤 1】删除测试 Pod (memhog-test)..."
if kubectl get pod memhog-test &>/dev/null; then
    kubectl delete pod memhog-test --wait=true --timeout=60s 2>&1
    echo "✅ memhog-test Pod 已删除"
    CLEANED=$((CLEANED + 1))
else
    echo "⏭️  memhog-test Pod 不存在，跳过"
    SKIPPED=$((SKIPPED + 1))
fi

# -----------------------------------------------
# 【步骤 2】卸载 Helm Release
# -----------------------------------------------
echo ""
echo "【步骤 2】卸载 Helm Release (etmem-operator)..."
if helm status etmem-operator -n etmem-system &>/dev/null; then
    helm uninstall etmem-operator -n etmem-system 2>&1
    echo "✅ Helm Release etmem-operator 已卸载"
    CLEANED=$((CLEANED + 1))

    # 等待资源清理
    echo "  等待资源清理（10 秒）..."
    sleep 10
else
    echo "⏭️  Helm Release etmem-operator 不存在，跳过"
    SKIPPED=$((SKIPPED + 1))
fi

# -----------------------------------------------
# 【步骤 3】删除命名空间
# -----------------------------------------------
echo ""
echo "【步骤 3】删除 etmem-system 命名空间..."
if kubectl get namespace etmem-system &>/dev/null; then
    kubectl delete ns etmem-system --ignore-not-found --timeout=120s 2>&1
    echo "✅ etmem-system 命名空间已删除"
    CLEANED=$((CLEANED + 1))
else
    echo "⏭️  etmem-system 命名空间不存在，跳过"
    SKIPPED=$((SKIPPED + 1))
fi

# -----------------------------------------------
# 【步骤 4】处理 CRD（需确认或 --force）
# -----------------------------------------------
echo ""
echo "【步骤 4】处理 etmem 相关 CRD..."

# 获取 etmem 相关 CRD 列表
ETMEM_CRDS=$(kubectl get crd 2>/dev/null | grep "etmem" | awk '{print $1}' || true)

if [ -n "${ETMEM_CRDS}" ]; then
    echo "  发现以下 etmem 相关 CRD："
    echo "${ETMEM_CRDS}" | while read -r crd; do
        echo "    - ${crd}"
    done
    echo ""

    if [ "${FORCE_DELETE_CRDS}" = true ]; then
        # 使用 --force 参数时直接删除
        echo "  使用 --force 模式，直接删除所有 etmem CRD..."
        echo "${ETMEM_CRDS}" | while read -r crd; do
            if [ -n "${crd}" ]; then
                kubectl delete crd "${crd}" --timeout=60s 2>&1 || true
                echo "  ✅ 已删除 CRD: ${crd}"
            fi
        done
        CLEANED=$((CLEANED + 1))
    else
        # 非 force 模式，检查是否在交互终端
        if [ -t 0 ]; then
            echo "  ⚠️  删除 CRD 将移除所有相关自定义资源数据！"
            read -r -p "  是否删除 etmem CRD？(y/N): " CONFIRM
            if [ "${CONFIRM}" = "y" ] || [ "${CONFIRM}" = "Y" ]; then
                echo "${ETMEM_CRDS}" | while read -r crd; do
                    if [ -n "${crd}" ]; then
                        kubectl delete crd "${crd}" --timeout=60s 2>&1 || true
                        echo "  ✅ 已删除 CRD: ${crd}"
                    fi
                done
                CLEANED=$((CLEANED + 1))
            else
                echo "  ⏭️  跳过 CRD 删除"
                SKIPPED=$((SKIPPED + 1))
            fi
        else
            echo "  ⏭️  非交互模式，跳过 CRD 删除（使用 --force 参数可强制删除）"
            SKIPPED=$((SKIPPED + 1))
        fi
    fi
else
    echo "⏭️  未发现 etmem 相关 CRD，跳过"
    SKIPPED=$((SKIPPED + 1))
fi

# -----------------------------------------------
# 【步骤 5】清理摘要
# -----------------------------------------------
echo ""
echo "=============================================="
echo " 清理完成"
echo "=============================================="
echo ""
echo "清理结束时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""
echo "📊 清理统计："
echo "   ✅ 已清理: ${CLEANED} 项"
echo "   ⏭️  已跳过: ${SKIPPED} 项"
echo ""

# 验证清理结果
echo "--- 验证清理结果 ---"
echo ""
echo "etmem-system 命名空间:"
if kubectl get namespace etmem-system &>/dev/null; then
    echo "  ⚠️  仍然存在（可能正在删除中）"
else
    echo "  ✅ 已不存在"
fi

echo ""
echo "memhog-test Pod:"
if kubectl get pod memhog-test &>/dev/null; then
    echo "  ⚠️  仍然存在"
else
    echo "  ✅ 已不存在"
fi

echo ""
echo "etmem CRD:"
REMAINING_CRDS=$(kubectl get crd 2>/dev/null | grep "etmem" || true)
if [ -n "${REMAINING_CRDS}" ]; then
    echo "  ⚠️  仍有 etmem CRD 存在（使用 --force 参数可强制删除）"
    echo "${REMAINING_CRDS}" | sed 's/^/    /'
else
    echo "  ✅ 已全部清理"
fi

echo ""
echo "💡 提示：若需要强制删除 CRD，请运行: $0 --force"
