#!/bin/bash
set -euo pipefail

# =============================================================================
# verify-existing-workload-etmem.sh
# 用途：验证现有工作负载的 etmem 管理状态（控制面 + 数据面）
# 用法：
#   sudo bash scripts/verify-existing-workload-etmem.sh <pod-name> [命名空间]
# 示例：
#   sudo bash scripts/verify-existing-workload-etmem.sh dbservice-xxxxx
#   sudo bash scripts/verify-existing-workload-etmem.sh dbservice-xxxxx default
# =============================================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
ARTIFACTS_DIR="${REPO_DIR}/artifacts"
mkdir -p "${ARTIFACTS_DIR}"

POD_NAME="${1:-}"
NAMESPACE="${2:-default}"
ETMEM_NAMESPACE="etmem-system"

if [ -z "$POD_NAME" ]; then
    echo "用法: sudo bash $0 <pod-name> [命名空间]"
    echo ""
    echo "参数说明："
    echo "  pod-name    要验证的 Pod 名称"
    echo "  命名空间    Pod 所在的命名空间（默认: default）"
    echo ""
    echo "示例："
    echo "  sudo bash $0 dbservice-xxxxx"
    echo "  sudo bash $0 myapp-abc123 my-namespace"
    exit 1
fi

RESULT_FILE="${ARTIFACTS_DIR}/verify-workload-${POD_NAME}.txt"
: > "${RESULT_FILE}"

log_result() {
    local msg="$1"
    echo "$msg" | tee -a "${RESULT_FILE}"
}

filter_agent_lines_for_pod() {
    local logs="$1"
    local pod_name="$2"
    echo "${logs}" | grep "\"pod\": \"${pod_name}\"" 2>/dev/null || true
}

build_cgroup_candidates() {
    local pod_uid="$1"
    local qos_class="$2"
    local pod_uid_under
    pod_uid_under=$(echo "$pod_uid" | tr '-' '_')

    case "$(echo "$qos_class" | tr '[:upper:]' '[:lower:]')" in
        guaranteed)
            cat <<EOF
/sys/fs/cgroup/memory/kubepods.slice/kubepods-pod${pod_uid_under}.slice
/sys/fs/cgroup/memory/kubepods/pod${pod_uid}
EOF
            ;;
        burstable)
            cat <<EOF
/sys/fs/cgroup/memory/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod${pod_uid_under}.slice
/sys/fs/cgroup/memory/kubepods/burstable/pod${pod_uid}
EOF
            ;;
        *)
            cat <<EOF
/sys/fs/cgroup/memory/kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-pod${pod_uid_under}.slice
/sys/fs/cgroup/memory/kubepods/besteffort/pod${pod_uid}
EOF
            ;;
    esac
}

# 最终判定计数器
CONTROL_PLANE_OK=0
DATA_PLANE_OK=0

echo "=============================================="
echo " 验证现有工作负载的 etmem 管理状态"
echo " $(date '+%Y-%m-%d %H:%M:%S')"
echo "=============================================="
echo ""
log_result "验证时间: $(date '+%Y-%m-%d %H:%M:%S')"
log_result "目标 Pod: ${POD_NAME}（命名空间: ${NAMESPACE}）"
log_result ""

# -----------------------------------------------
# 【步骤 1】检查 Pod 是否存在及标签
# -----------------------------------------------
echo "【步骤 1】检查 Pod 状态和标签..."
POD_PHASE=$(kubectl get pod "${POD_NAME}" -n "${NAMESPACE}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")

if [ -z "$POD_PHASE" ]; then
    echo -e "  ${RED}❌ 未找到 Pod ${POD_NAME}（命名空间: ${NAMESPACE}）${NC}"
    log_result "❌ Pod 不存在"
    exit 1
fi

echo "  Pod 状态: ${POD_PHASE}"
log_result "Pod 状态: ${POD_PHASE}"

# 检查 etmem 标签
ETMEM_LABEL=$(kubectl get pod "${POD_NAME}" -n "${NAMESPACE}" \
    -o jsonpath='{.metadata.labels.etmem\.openeuler\.io/enable}' 2>/dev/null || echo "")

if [ "$ETMEM_LABEL" = "true" ]; then
    echo -e "  ${GREEN}✅ etmem 标签已设置: etmem.openeuler.io/enable=true${NC}"
    log_result "✅ etmem 标签: 已设置"
else
    echo -e "  ${YELLOW}⚠️  etmem 标签未设置（当前值: '${ETMEM_LABEL}'）${NC}"
    echo "  提示: 使用 scripts/patch-existing-workload-for-etmem.sh 添加标签"
    log_result "⚠️ etmem 标签: 未设置"
fi
echo ""

# -----------------------------------------------
# 【步骤 2】检查 Agent 日志中的匹配信息
# -----------------------------------------------
echo "【步骤 2】检查 Agent 日志中的 Pod 匹配信息..."
log_result ""
log_result "--- Agent 日志匹配信息 ---"

AGENT_LOGS=$(kubectl -n "${ETMEM_NAMESPACE}" logs ds/etmem-agent --tail=500 2>&1 || echo "无法获取 Agent 日志")
POD_AGENT_LOGS=$(filter_agent_lines_for_pod "${AGENT_LOGS}" "${POD_NAME}")

MATCH_LINES=$(echo "${POD_AGENT_LOGS}" | grep -i "matched pod" 2>/dev/null || true)

if [ -n "$MATCH_LINES" ]; then
    echo -e "  ${GREEN}✅ Agent 已匹配到目标 Pod${NC}"
    echo "$MATCH_LINES" | head -10 | sed 's/^/    /' | tee -a "${RESULT_FILE}"
    CONTROL_PLANE_OK=1
    log_result "✅ 控制面：Agent 已匹配 Pod"
else
    echo -e "  ${YELLOW}⚠️  Agent 日志中未找到对 ${POD_NAME} 的匹配记录${NC}"
    log_result "⚠️ 控制面：未找到匹配记录"
fi

# 检查项目创建信息
PROJECT_LINES=$(echo "${POD_AGENT_LOGS}" | grep -i "task started successfully\|project" 2>/dev/null || true)
if [ -n "$PROJECT_LINES" ]; then
    echo ""
    echo "  项目/PID 相关日志:"
    echo "$PROJECT_LINES" | head -10 | sed 's/^/    /' | tee -a "${RESULT_FILE}"
fi
echo ""

# -----------------------------------------------
# 【步骤 3】检查 EtmemNodeState
# -----------------------------------------------
echo "【步骤 3】检查 EtmemNodeState..."
log_result ""
log_result "--- EtmemNodeState ---"

POD_NODE=$(kubectl get pod "${POD_NAME}" -n "${NAMESPACE}" -o jsonpath='{.spec.nodeName}' 2>/dev/null || echo "")
NODESTATE_TASKS=$(kubectl get etmemnodestate -A -o jsonpath='{range .items[*].status.tasks[*]}{.podName} → {.projectName} ({.state}){"\n"}{end}' 2>/dev/null || echo "")
NODESTATE_HEALTH=""

if [ -n "$POD_NODE" ]; then
    NODESTATE_HEALTH=$(kubectl get etmemnodestate "${POD_NODE}" -o jsonpath='{.status.socketReachable} {.status.etmemdReady} {.status.metrics.totalManagedPods}' 2>/dev/null || echo "")
fi

if [ -n "$NODESTATE_HEALTH" ]; then
    SOCKET_REACHABLE=$(echo "$NODESTATE_HEALTH" | awk '{print $1}')
    ETMEMD_READY=$(echo "$NODESTATE_HEALTH" | awk '{print $2}')
    MANAGED_PODS=$(echo "$NODESTATE_HEALTH" | awk '{print $3}')
    echo "  NodeState 健康字段:"
    echo "    socketReachable=${SOCKET_REACHABLE}"
    echo "    etmemdReady=${ETMEMD_READY}"
    echo "    totalManagedPods=${MANAGED_PODS}"
    log_result "NodeState 健康字段: socketReachable=${SOCKET_REACHABLE}, etmemdReady=${ETMEMD_READY}, totalManagedPods=${MANAGED_PODS}"
fi

if [ -n "$NODESTATE_TASKS" ]; then
    POD_TASKS=$(echo "$NODESTATE_TASKS" | grep "${POD_NAME}" || true)
    if [ -n "$POD_TASKS" ]; then
        echo -e "  ${GREEN}✅ NodeState 中存在目标 Pod 的任务记录${NC}"
        echo "$POD_TASKS" | sed 's/^/    /' | tee -a "${RESULT_FILE}"
        TASK_COUNT=$(echo "$POD_TASKS" | wc -l)
        echo "  任务数: ${TASK_COUNT}"
        log_result "✅ NodeState：发现 ${TASK_COUNT} 个任务"
        CONTROL_PLANE_OK=1
    else
        echo -e "  ${YELLOW}⚠️  NodeState 中未找到 ${POD_NAME} 的任务记录${NC}"
        log_result "⚠️ NodeState：未找到目标 Pod 的任务"
    fi
else
    echo -e "  ${YELLOW}⚠️  无法获取 EtmemNodeState 或无活跃任务${NC}"
    log_result "⚠️ NodeState：无活跃任务"
fi
echo ""

# -----------------------------------------------
# 【步骤 4】查找 Pod 内所有用户进程
# -----------------------------------------------
echo "【步骤 4】查找 Pod 内的用户进程..."
log_result ""
log_result "--- Pod 进程列表 ---"

POD_UID=$(kubectl get pod "${POD_NAME}" -n "${NAMESPACE}" -o jsonpath='{.metadata.uid}' 2>/dev/null || echo "")

TARGET_PIDS=()

if [ -n "$POD_UID" ]; then
    CGROUP_FOUND=""
    POD_QOS=$(kubectl get pod "${POD_NAME}" -n "${NAMESPACE}" -o jsonpath='{.status.qosClass}' 2>/dev/null || echo "")
    declare -A PID_SEEN=()

    while read -r CGROUP_BASE; do
        [ -n "$CGROUP_BASE" ] || continue
        if [ -d "$CGROUP_BASE" ]; then
            CGROUP_FOUND="$CGROUP_BASE"
            echo "  cgroup 路径: ${CGROUP_BASE}"
            log_result "cgroup 路径: ${CGROUP_BASE}"
            if [ -f "$CGROUP_BASE/cgroup.procs" ]; then
                while read -r pid; do
                    [ -n "${PID_SEEN[$pid]:-}" ] && continue
                    comm=$(cat "/proc/$pid/comm" 2>/dev/null || echo "unknown")
                    if [ "$comm" != "pause" ] && [ "$comm" != "sandbox" ]; then
                        TARGET_PIDS+=("$pid")
                        PID_SEEN["$pid"]=1
                    fi
                done < "$CGROUP_BASE/cgroup.procs"
            fi

            for scope in "$CGROUP_BASE"/*; do
                [ -d "$scope" ] || continue
                [ -f "$scope/cgroup.procs" ] || continue
                while read -r pid; do
                    [ -n "${PID_SEEN[$pid]:-}" ] && continue
                    comm=$(cat "/proc/$pid/comm" 2>/dev/null || echo "unknown")
                    if [ "$comm" != "pause" ] && [ "$comm" != "sandbox" ]; then
                        TARGET_PIDS+=("$pid")
                        PID_SEEN["$pid"]=1
                    fi
                done < "$scope/cgroup.procs"
            done
            break
        fi
    done < <(build_cgroup_candidates "$POD_UID" "$POD_QOS")

    if [ -z "$CGROUP_FOUND" ]; then
        echo -e "  ${YELLOW}⚠️  未找到 Pod cgroup 路径（可能需要 root 权限或 cgroup 版本不同）${NC}"
        log_result "⚠️ 未找到 cgroup 路径（QOS=${POD_QOS}）"
    fi
else
    echo -e "  ${RED}❌ 无法获取 Pod UID${NC}"
    log_result "❌ 无法获取 Pod UID"
fi

echo "  发现 ${#TARGET_PIDS[@]} 个用户进程（已排除 pause）"
log_result "用户进程数: ${#TARGET_PIDS[@]}"
echo ""

# -----------------------------------------------
# 【步骤 5】检查各进程 VmRSS / VmSwap
# -----------------------------------------------
echo "【步骤 5】各进程内存状态（VmRSS / VmSwap）..."
log_result ""
log_result "--- 进程内存状态 ---"

if [ ${#TARGET_PIDS[@]} -gt 0 ]; then
    TOTAL_RSS=0
    TOTAL_SWAP=0
    SWAP_OK_COUNT=0

    HEADER=$(printf "  %-10s %-20s %12s %12s %s" "PID" "进程名" "VmRSS(kB)" "VmSwap(kB)" "状态")
    echo "$HEADER"
    log_result "$HEADER"
    DIVIDER=$(printf "  %-10s %-20s %12s %12s %s" "----------" "--------------------" "------------" "------------" "------")
    echo "$DIVIDER"
    log_result "$DIVIDER"

    for pid in "${TARGET_PIDS[@]}"; do
        if [ -f "/proc/$pid/status" ]; then
            comm=$(cat "/proc/$pid/comm" 2>/dev/null || echo "N/A")
            rss=$(grep VmRSS "/proc/$pid/status" 2>/dev/null | awk '{print $2}' || echo "0")
            swap=$(grep VmSwap "/proc/$pid/status" 2>/dev/null | awk '{print $2}' || echo "0")

            if [ "${swap:-0}" -gt 0 ] 2>/dev/null; then
                STATUS="${GREEN}✅${NC}"
                STATUS_TEXT="✅"
                SWAP_OK_COUNT=$((SWAP_OK_COUNT + 1))
            else
                STATUS="${YELLOW}⚠️${NC}"
                STATUS_TEXT="⚠️"
            fi

            LINE=$(printf "  %-10s %-20s %12s %12s" "$pid" "$comm" "${rss:-0}" "${swap:-0}")
            echo -e "${LINE} ${STATUS}"
            log_result "$(printf "  %-10s %-20s %12s %12s %s" "$pid" "$comm" "${rss:-0}" "${swap:-0}" "$STATUS_TEXT")"

            TOTAL_RSS=$((TOTAL_RSS + ${rss:-0}))
            TOTAL_SWAP=$((TOTAL_SWAP + ${swap:-0}))
        else
            LINE=$(printf "  %-10s %-20s %12s %12s" "$pid" "进程不存在" "-" "-")
            echo -e "${LINE} ${RED}❌${NC}"
            log_result "$(printf "  %-10s %-20s %12s %12s %s" "$pid" "进程不存在" "-" "-" "❌")"
        fi
    done

    echo ""
    echo "  ─── 汇总 ───"
    echo "  进程总数:           ${#TARGET_PIDS[@]}"
    echo "  VmSwap > 0 的进程:  ${SWAP_OK_COUNT}"
    echo "  VmRSS 合计:         ${TOTAL_RSS} kB ($((TOTAL_RSS / 1024)) MB)"
    echo "  VmSwap 合计:        ${TOTAL_SWAP} kB ($((TOTAL_SWAP / 1024)) MB)"

    log_result ""
    log_result "汇总: 进程总数=${#TARGET_PIDS[@]}, VmSwap>0=${SWAP_OK_COUNT}, RSS合计=${TOTAL_RSS}kB, Swap合计=${TOTAL_SWAP}kB"

    if [ "$TOTAL_SWAP" -gt 0 ]; then
        DATA_PLANE_OK=1
    fi
else
    echo -e "  ${YELLOW}⚠️  未发现用户进程，无法检查内存状态${NC}"
    echo "  提示: 此步骤需要在 Pod 所在节点上以 root 权限运行"
    log_result "⚠️ 未发现用户进程"
fi
echo ""

# -----------------------------------------------
# 【步骤 5.5】补充主机级内存观测
# -----------------------------------------------
echo "【步骤 5.5】主机级内存观测（补充）..."
log_result ""
log_result "--- 主机级内存观测 ---"

FREE_M=$(free -m 2>/dev/null || true)
MEM_AVAILABLE_KB=$(grep '^MemAvailable:' /proc/meminfo 2>/dev/null | awk '{print $2}' || echo "")

if [ -n "$FREE_M" ]; then
    echo "$FREE_M" | sed 's/^/  /' | tee -a "${RESULT_FILE}"
fi
if [ -n "$MEM_AVAILABLE_KB" ]; then
    echo "  MemAvailable: ${MEM_AVAILABLE_KB} kB" | tee -a "${RESULT_FILE}"
fi
echo ""

# -----------------------------------------------
# 【步骤 6】综合判定
# -----------------------------------------------
echo "=============================================="
echo " 综合判定"
echo "=============================================="
echo ""
log_result ""
log_result "=== 综合判定 ==="

# 控制面判定
if [ "$CONTROL_PLANE_OK" -eq 1 ]; then
    echo -e "  控制面: ${GREEN}✅ 通过${NC}"
    echo "    Agent 已匹配到目标 Pod，NodeState 中存在对应任务"
    log_result "控制面: ✅ 通过"
else
    if [ "$ETMEM_LABEL" = "true" ]; then
        echo -e "  控制面: ${YELLOW}⚠️  待确认${NC}"
        echo "    标签已设置但未检测到 Agent 匹配，可能需要等待 reconcile（30-60 秒）"
        log_result "控制面: ⚠️ 待确认"
    else
        echo -e "  控制面: ${RED}❌ 未启用${NC}"
        echo "    Pod 未设置 etmem 标签"
        log_result "控制面: ❌ 未启用"
    fi
fi

# 数据面判定
if [ "$DATA_PLANE_OK" -eq 1 ]; then
    echo -e "  数据面: ${GREEN}✅ 通过${NC}"
    echo "    检测到进程 VmSwap > 0，冷页已被换出"
    log_result "数据面: ✅ 通过"
else
    if [ ${#TARGET_PIDS[@]} -gt 0 ]; then
        echo -e "  数据面: ${YELLOW}⚠️  未生效${NC}"
        echo "    所有进程 VmSwap = 0，可能原因："
        echo "      - etmem 尚未完成首次 reconcile（等待 30-60 秒后重试）"
        echo "      - sysmem_threshold 未触发（大内存机器需 aggressive/extreme profile）"
        echo "      - 内核模块未加载（检查 etmem_scan、etmem_swap）"
        echo "      - etmemd 未运行"
        log_result "数据面: ⚠️ 未生效"
    else
        echo -e "  数据面: ${YELLOW}⚠️  无法验证${NC}"
        echo "    未发现用户进程，请确认在 Pod 所在节点上以 root 权限运行"
        log_result "数据面: ⚠️ 无法验证"
    fi
fi

echo ""
log_result ""
log_result "验证完成时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "详细结果已保存至: ${RESULT_FILE}"
