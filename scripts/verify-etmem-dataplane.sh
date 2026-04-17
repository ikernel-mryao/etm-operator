#!/bin/bash
# ============================================================
# etmem 数据面验证脚本（支持多进程）
# 用途：自动检查关键环境状态、配置、多进程 swap 情况
# 运行方式：
#   sudo bash scripts/verify-etmem-dataplane.sh [PID ...]
#   sudo bash scripts/verify-etmem-dataplane.sh --pod <pod-name>
# ============================================================
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SOCKET_NAME="${ETMEMD_SOCKET:-etmemd_socket}"
POD_MODE=""
POD_NAME=""
TARGET_PIDS=()

# 参数解析：支持 --pod <name> 或直接传 PID 列表
while [[ $# -gt 0 ]]; do
    case "$1" in
        --pod)
            POD_MODE="1"
            POD_NAME="${2:-}"
            shift 2
            ;;
        *)
            TARGET_PIDS+=("$1")
            shift
            ;;
    esac
done

# 兼容旧用法：单个 PID
TARGET_PID="${TARGET_PIDS[0]:-}"

echo "========================================"
echo " etmem 数据面验证诊断"
echo " $(date)"
echo "========================================"
echo ""

# ------- 1. 内核模块检查 -------
echo "【1】内核模块检查"
for mod in etmem_scan etmem_swap; do
    if lsmod | grep -q "$mod"; then
        echo -e "  ${GREEN}✅ $mod 已加载${NC}"
    else
        echo -e "  ${RED}❌ $mod 未加载${NC} — 请执行: sudo insmod /path/to/$mod.ko"
    fi
done
echo ""

# ------- 2. swap 状态检查 -------
echo "【2】Swap 状态检查"
SWAP_TOTAL=$(grep SwapTotal /proc/meminfo | awk '{print $2}')
SWAP_FREE=$(grep SwapFree /proc/meminfo | awk '{print $2}')
if [ "$SWAP_TOTAL" -gt 0 ]; then
    SWAP_USED=$((SWAP_TOTAL - SWAP_FREE))
    echo -e "  ${GREEN}✅ Swap 已启用${NC}: Total=${SWAP_TOTAL}kB, Used=${SWAP_USED}kB, Free=${SWAP_FREE}kB"
else
    echo -e "  ${RED}❌ Swap 未启用${NC} — 请配置 swap 分区或 swap 文件"
fi
echo ""

# ------- 3. etmemd 进程检查 -------
echo "【3】etmemd 进程检查"
ETMEMD_PID=$(pgrep -f '/usr/bin/etmemd' | head -1 || true)
if [ -n "$ETMEMD_PID" ]; then
    echo -e "  ${GREEN}✅ etmemd 正在运行${NC} (PID: $ETMEMD_PID)"
else
    echo -e "  ${RED}❌ etmemd 未运行${NC} — 请执行: sudo nohup /usr/bin/etmemd -s $SOCKET_NAME -l 2 &"
fi
echo ""

# ------- 4. etmemd 项目状态 -------
echo "【4】etmemd 项目状态"
PROJECT_OUT=$(/usr/bin/etmem project show -s "$SOCKET_NAME" 2>&1 || true)
echo "$PROJECT_OUT" | sed 's/^/  /'
if echo "$PROJECT_OUT" | grep -q "true"; then
    PROJ_COUNT=$(echo "$PROJECT_OUT" | grep -c "^project:" || echo 0)
    echo -e "  ${GREEN}✅ 有 $PROJ_COUNT 个活跃项目${NC}"
else
    echo -e "  ${YELLOW}⚠️  没有活跃项目或项目未启动${NC}"
fi
echo ""

# ------- 5. 系统内存与阈值分析 -------
echo "【5】系统内存与阈值分析"
MEM_TOTAL=$(grep MemTotal /proc/meminfo | awk '{print $2}')
MEM_AVAIL=$(grep MemAvailable /proc/meminfo | awk '{print $2}')
AVAIL_PCT=$(echo "scale=1; $MEM_AVAIL * 100 / $MEM_TOTAL" | bc)
echo "  MemTotal:     ${MEM_TOTAL} kB"
echo "  MemAvailable: ${MEM_AVAIL} kB"
echo "  可用比例:     ${AVAIL_PCT}%"
echo ""
echo "  阈值触发条件说明:"
echo '  sysmem_threshold=N 表示"当可用内存 < N% 时才触发 swap"'
echo "  当前可用 ${AVAIL_PCT}%，需要 threshold > ${AVAIL_PCT} 才能触发"
echo ""
echo "  各 profile 的 threshold 值:"
echo "    conservative = 85  $([ $(echo "$AVAIL_PCT < 85" | bc) -eq 1 ] && echo -e "${GREEN}✅ 会触发${NC}" || echo -e "${RED}❌ 不会触发${NC}")"
echo "    moderate     = 90  $([ $(echo "$AVAIL_PCT < 90" | bc) -eq 1 ] && echo -e "${GREEN}✅ 会触发${NC}" || echo -e "${RED}❌ 不会触发${NC}")"
echo "    aggressive   = 95  $([ $(echo "$AVAIL_PCT < 95" | bc) -eq 1 ] && echo -e "${GREEN}✅ 会触发${NC}" || echo -e "${RED}❌ 不会触发${NC}")"
echo "    extreme      = 99  $([ $(echo "$AVAIL_PCT < 99" | bc) -eq 1 ] && echo -e "${GREEN}✅ 会触发${NC}" || echo -e "${RED}❌ 不会触发${NC}")"
echo ""

# ------- 6. 配置文件检查 -------
echo "【6】etmem 配置文件检查"
CONF_DIR="/var/run/etmem/configs"
if [ -d "$CONF_DIR" ]; then
    CONF_FILES=$(ls "$CONF_DIR"/*.conf 2>/dev/null || true)
    if [ -n "$CONF_FILES" ]; then
        for f in $CONF_FILES; do
            echo "  --- $f ---"
            PERMS=$(stat -c '%a' "$f")
            if [ "$PERMS" = "600" ] || [ "$PERMS" = "400" ]; then
                echo -e "  权限: ${GREEN}$PERMS ✅${NC}"
            else
                echo -e "  权限: ${RED}$PERMS ❌${NC} — etmemd 要求 600 或 400"
            fi
            # 检查 [task] 数量
            TASK_COUNT=$(grep -c '^\[task\]' "$f" || true)
            if [ "$TASK_COUNT" -gt 1 ]; then
                echo -e "  [task] 段数: ${RED}$TASK_COUNT ❌${NC} — etmemd 只支持最后一个 [task]"
            else
                echo -e "  [task] 段数: ${GREEN}$TASK_COUNT ✅${NC}"
            fi
            # 检查 task value
            TASK_VALUE=$(grep '^value=' "$f" | tail -1 | cut -d= -f2)
            echo "  目标进程: $TASK_VALUE"
            THRESHOLD=$(grep '^sysmem_threshold=' "$f" | cut -d= -f2)
            echo "  sysmem_threshold: $THRESHOLD"
            echo ""
        done
    else
        echo -e "  ${YELLOW}⚠️  无配置文件${NC}"
    fi
else
    echo -e "  ${YELLOW}⚠️  配置目录不存在${NC}"
fi
echo ""

# ------- 7. 多进程 VmSwap 汇总检查 -------
echo "【7】目标进程 VmSwap 检查（支持多进程）"

# 如果使用 --pod 模式，自动发现 Pod 内所有进程的宿主机 PID
if [ -n "$POD_MODE" ] && [ -n "$POD_NAME" ]; then
    echo -e "  ${BLUE}Pod 模式：自动发现 $POD_NAME 内的宿主机 PID${NC}"
    POD_UID=$(kubectl get pod "$POD_NAME" -o jsonpath='{.metadata.uid}' 2>/dev/null || echo "")
    if [ -z "$POD_UID" ]; then
        echo -e "  ${RED}❌ 无法获取 Pod UID，请检查 Pod 名称${NC}"
    else
        POD_UID_UNDER=$(echo "$POD_UID" | tr '-' '_')
        # 尝试 burstable 和 besteffort QoS 类别
        for QOS in burstable besteffort guaranteed; do
            CGROUP_BASE="/sys/fs/cgroup/memory/kubepods.slice/kubepods-${QOS}.slice/kubepods-${QOS}-pod${POD_UID_UNDER}.slice"
            if [ -d "$CGROUP_BASE" ]; then
                echo "  cgroup 路径: $CGROUP_BASE"
                for scope in "$CGROUP_BASE"/cri-containerd-*.scope; do
                    [ -d "$scope" ] || continue
                    while read pid; do
                        comm=$(cat /proc/$pid/comm 2>/dev/null || echo "unknown")
                        # 排除 pause 基础设施进程
                        if [ "$comm" != "pause" ]; then
                            TARGET_PIDS+=("$pid")
                        fi
                    done < "$scope/cgroup.procs"
                done
                break
            fi
        done
        echo "  发现 ${#TARGET_PIDS[@]} 个用户进程"
    fi
fi

if [ ${#TARGET_PIDS[@]} -gt 0 ]; then
    TOTAL_SWAP=0
    TOTAL_RSS=0
    SWAP_OK=0
    echo ""
    printf "  %-10s %-15s %12s %12s %s\n" "PID" "进程名" "VmRSS(kB)" "VmSwap(kB)" "状态"
    printf "  %-10s %-15s %12s %12s %s\n" "----------" "---------------" "------------" "------------" "------"
    for pid in "${TARGET_PIDS[@]}"; do
        if [ -f "/proc/$pid/status" ]; then
            comm=$(cat /proc/$pid/comm 2>/dev/null || echo "N/A")
            rss=$(grep VmRSS "/proc/$pid/status" 2>/dev/null | awk '{print $2}' || echo "0")
            swap=$(grep VmSwap "/proc/$pid/status" 2>/dev/null | awk '{print $2}' || echo "0")
            if [ "$swap" -gt 0 ] 2>/dev/null; then
                STATUS="${GREEN}✅${NC}"
                SWAP_OK=$((SWAP_OK + 1))
            else
                STATUS="${YELLOW}⚠️${NC}"
            fi
            printf "  %-10s %-15s %12s %12s " "$pid" "$comm" "$rss" "$swap"
            echo -e "$STATUS"
            TOTAL_RSS=$((TOTAL_RSS + ${rss:-0}))
            TOTAL_SWAP=$((TOTAL_SWAP + ${swap:-0}))
        else
            printf "  %-10s %-15s %12s %12s " "$pid" "不存在" "-" "-"
            echo -e "${RED}❌${NC}"
        fi
    done
    echo ""
    echo "  ─── 汇总 ───"
    echo "  进程总数: ${#TARGET_PIDS[@]}"
    echo "  VmSwap > 0 的进程: $SWAP_OK"
    echo "  VmRSS 合计: $TOTAL_RSS kB ($((TOTAL_RSS / 1024)) MB)"
    echo "  VmSwap 合计: $TOTAL_SWAP kB ($((TOTAL_SWAP / 1024)) MB)"
    if [ "$SWAP_OK" -eq "${#TARGET_PIDS[@]}" ] && [ "$TOTAL_SWAP" -gt 0 ]; then
        echo -e "  ${GREEN}✅ 所有进程均已触发 swap — 多进程数据面验证通过${NC}"
    elif [ "$SWAP_OK" -gt 0 ]; then
        echo -e "  ${YELLOW}⚠️  部分进程触发 swap（$SWAP_OK/${#TARGET_PIDS[@]}）${NC}"
    else
        echo -e "  ${YELLOW}⚠️  所有进程 VmSwap = 0 — 数据面尚未生效${NC}"
    fi
elif [ -n "$TARGET_PID" ]; then
    # 向后兼容：单 PID 模式
    if [ -f "/proc/$TARGET_PID/status" ]; then
        echo "  PID: $TARGET_PID"
        grep -E 'Name|VmRSS|VmSwap|VmSize' "/proc/$TARGET_PID/status" | while read line; do
            echo "  $line"
        done
        VMSWAP=$(grep VmSwap "/proc/$TARGET_PID/status" | awk '{print $2}')
        if [ "$VMSWAP" -gt 0 ]; then
            echo -e "  ${GREEN}✅ VmSwap > 0: 数据面生效${NC}"
        else
            echo -e "  ${YELLOW}⚠️  VmSwap = 0: 数据面尚未生效${NC}"
        fi
    else
        echo -e "  ${RED}❌ PID $TARGET_PID 不存在${NC}"
    fi
else
    echo "  未指定 PID 或 Pod，跳过"
    echo "  用法:"
    echo "    $0 <PID> [PID2 ...]       # 检查指定 PID"
    echo "    $0 --pod <pod-name>        # 自动发现 Pod 内所有进程"
    echo "  提示: --pod 模式会自动排除 pause 基础设施进程"
fi
echo ""

# ------- 8. 多项目一致性检查 -------
echo "【8】etmemd 多项目一致性检查"
PROJECT_COUNT=$(etmem project show -s "$SOCKET_NAME" 2>/dev/null | grep -c "^project:" || echo 0)
echo "  当前活跃项目数: $PROJECT_COUNT"
if [ "$PROJECT_COUNT" -gt 0 ]; then
    etmem project show -s "$SOCKET_NAME" 2>/dev/null | grep "^project:" | while read line; do
        proj=$(echo "$line" | awk '{print $2}')
        echo -e "  ${GREEN}▸${NC} $proj"
    done
    echo ""
    # 检查项目数 vs 配置文件数是否一致
    CONFIG_COUNT=$(ls /var/run/etmem/configs/*.conf 2>/dev/null | wc -l || echo 0)
    if [ "$PROJECT_COUNT" -eq "$CONFIG_COUNT" ]; then
        echo -e "  ${GREEN}✅ 项目数 ($PROJECT_COUNT) = 配置文件数 ($CONFIG_COUNT)${NC}"
    else
        echo -e "  ${YELLOW}⚠️  项目数 ($PROJECT_COUNT) ≠ 配置文件数 ($CONFIG_COUNT) — 可能存在孤儿项目或遗漏配置${NC}"
    fi
fi
echo ""

# ------- 旧的同名进程干扰检查（保留兼容） -------
if [ -n "$TARGET_PID" ] && [ -f "/proc/$TARGET_PID/comm" ]; then
    echo "【附加】同名进程干扰检查"
    PROC_NAME=$(cat "/proc/$TARGET_PID/comm")
    MATCHES=$(pgrep -c "$PROC_NAME" 2>/dev/null || echo 0)
    echo "  进程名: $PROC_NAME"
    echo "  同名进程数: $MATCHES"
    if [ "$MATCHES" -gt 1 ]; then
        echo -e "  ${YELLOW}⚠️  存在多个同名进程，type=name 模式可能受干扰${NC}"
        echo "  同名进程列表:"
        pgrep -a "$PROC_NAME" | while read line; do
            echo "    $line"
        done
    else
        echo -e "  ${GREEN}✅ 无同名进程干扰${NC}"
    fi
    echo ""
fi

# ------- 9. Kubernetes 组件状态 -------
echo "【9】Kubernetes 组件状态"
echo "  --- etmem-operator ---"
kubectl -n etmem-system get deploy etmem-operator --no-headers 2>&1 | sed 's/^/  /'
echo "  --- etmem-agent ---"
kubectl -n etmem-system get ds etmem-agent --no-headers 2>&1 | sed 's/^/  /'
echo "  --- EtmemPolicy ---"
kubectl get etmempolicy -A --no-headers 2>&1 | sed 's/^/  /'
echo "  --- EtmemNodeState ---"
kubectl get etmemnodestate -A --no-headers 2>&1 | sed 's/^/  /'
echo "  --- NodeState 任务列表 ---"
TASK_LIST=$(kubectl get etmemnodestate -A -o jsonpath='{range .items[*].status.tasks[*]}{.projectName} → {.podName} ({.state}){"\n"}{end}' 2>/dev/null || echo "无法获取")
if [ -n "$TASK_LIST" ]; then
    echo "$TASK_LIST" | while read line; do
        echo "    $line"
    done
else
    echo "    无活跃任务"
fi
echo ""

echo "========================================"
echo " 诊断完成"
echo "========================================"
