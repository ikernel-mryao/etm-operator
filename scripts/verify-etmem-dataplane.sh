#!/bin/bash
# ============================================================
# etmem 数据面验证脚本
# 用途：自动检查关键环境状态、配置、进程 swap 情况
# 运行方式：sudo bash scripts/verify-etmem-dataplane.sh [PID]
# ============================================================
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

SOCKET_NAME="${ETMEMD_SOCKET:-etmemd_socket}"
TARGET_PID="${1:-}"

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
echo "  $PROJECT_OUT"
if echo "$PROJECT_OUT" | grep -q "started.*true"; then
    echo -e "  ${GREEN}✅ 有活跃项目${NC}"
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
echo "  sysmem_threshold=N 表示"当可用内存 < N% 时才触发 swap""
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

# ------- 7. 目标进程 VmSwap 检查 -------
echo "【7】目标进程 VmSwap 检查"
if [ -n "$TARGET_PID" ]; then
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
    echo "  未指定 PID，跳过（用法: $0 <PID>）"
    echo "  提示: 可通过以下命令查找目标 PID:"
    echo "    sudo crictl ps --name <容器名> -q | head -1"
    echo "    sudo crictl inspect <CONTAINER_ID> | python3 -c \"import sys,json; print(json.load(sys.stdin)['info']['pid'])\""
fi
echo ""

# ------- 8. 同名进程干扰检查 -------
echo "【8】同名进程干扰检查"
if [ -n "$TARGET_PID" ] && [ -f "/proc/$TARGET_PID/comm" ]; then
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
fi
echo ""

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
echo ""

echo "========================================"
echo " 诊断完成"
echo "========================================"
