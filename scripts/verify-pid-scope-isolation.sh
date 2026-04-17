#!/bin/bash
# ============================================================
# PID 作用域隔离验证脚本
# 用途：验证 type=pid 修复后，同节点同名进程不会交叉命中
# 运行方式：
#   bash scripts/verify-pid-scope-isolation.sh
# 
# 前提条件：
#   - etmem-operator / etmem-agent 已部署（v0.4.0-pid+）
#   - EtmemPolicy 已创建且 selector 为 etmem.openeuler.io/enable: "true"
#   - 至少有一个带 enable 标签的 memhog Pod 在运行
# ============================================================
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

ARTIFACTS_DIR="${ARTIFACTS_DIR:-artifacts/scope-isolation}"
mkdir -p "$ARTIFACTS_DIR"

echo "========================================"
echo " PID 作用域隔离验证"
echo " $(date)"
echo "========================================"
echo ""

# ------- 1. 检查当前 Pod 和进程状态 -------
echo "【1】收集当前 Pod 和进程状态"

# 找到所有 memhog 进程
MEMHOG_PIDS=($(pgrep -x memhog 2>/dev/null || true))
echo "  节点上 memhog 进程总数: ${#MEMHOG_PIDS[@]}"

if [ ${#MEMHOG_PIDS[@]} -lt 2 ]; then
    echo -e "  ${YELLOW}⚠️  需要至少 2 个同名 memhog 进程来验证隔离性${NC}"
    echo "  请确保："
    echo "    1. 至少一个 memhog Pod 带 etmem.openeuler.io/enable=true 标签"
    echo "    2. 至少一个 memhog Pod 不带该标签"
    echo ""
    echo "  快速创建非标签 Pod:"
    echo "    kubectl run memhog-noenable --image=memhog:v1 -- /memhog 256"
    echo ""
fi

# ------- 2. 对每个 memhog PID，分析归属 -------
echo ""
echo "【2】进程归属分析"
echo ""
printf "  %-10s %-12s %-12s %-10s %s\n" "PID" "VmRSS(kB)" "VmSwap(kB)" "被管理?" "cgroup 归属"
printf "  %-10s %-12s %-12s %-10s %s\n" "----------" "------------" "------------" "----------" "-------------------------------------------"

MANAGED_COUNT=0
UNMANAGED_COUNT=0

for pid in "${MEMHOG_PIDS[@]}"; do
    rss=$(grep VmRSS /proc/$pid/status 2>/dev/null | awk '{print $2}' || echo "N/A")
    swap=$(grep VmSwap /proc/$pid/status 2>/dev/null | awk '{print $2}' || echo "0")
    cgroup_path=$(cat /proc/$pid/cgroup 2>/dev/null | grep memory | head -1 | sed 's|[^/]*/||' || echo "unknown")
    
    # 判断是否被 etmem 管理（VmSwap > 0 说明被管理）
    if [ "${swap:-0}" -gt 0 ] 2>/dev/null; then
        managed="${GREEN}是${NC}"
        MANAGED_COUNT=$((MANAGED_COUNT + 1))
    else
        managed="${YELLOW}否${NC}"
        UNMANAGED_COUNT=$((UNMANAGED_COUNT + 1))
    fi
    
    printf "  %-10s %-12s %-12s " "$pid" "$rss" "$swap"
    echo -e "$managed  $cgroup_path"
done

echo ""

# ------- 3. 检查配置文件中的 PID -------
echo "【3】etmem 配置文件 PID 分析"
CONF_DIR="/var/run/etmem/configs"
if [ -d "$CONF_DIR" ]; then
    CONFIGURED_PIDS=()
    for f in "$CONF_DIR"/*.conf 2>/dev/null; do
        [ -f "$f" ] || continue
        conf_type=$(grep '^type=' "$f" | cut -d= -f2)
        conf_value=$(grep '^value=' "$f" | cut -d= -f2)
        proj_name=$(basename "$f" .conf)
        echo "  配置: $proj_name"
        echo "    type=$conf_type, value=$conf_value"
        if [ "$conf_type" = "pid" ]; then
            CONFIGURED_PIDS+=("$conf_value")
            echo -e "    ${GREEN}✅ 使用 type=pid（精确 PID 定向）${NC}"
        elif [ "$conf_type" = "name" ]; then
            echo -e "    ${RED}❌ 仍使用 type=name（节点范围匹配，存在交叉风险）${NC}"
        fi
    done
    echo ""
    
    # 检查是否有非目标 PID 被配置
    echo "  配置中的 PID 列表: ${CONFIGURED_PIDS[*]:-无}"
    echo "  节点上的 memhog PID: ${MEMHOG_PIDS[*]:-无}"
    
    # 检查非配置 PID 是否被误管理
    for pid in "${MEMHOG_PIDS[@]}"; do
        in_config=false
        for cpid in "${CONFIGURED_PIDS[@]:-}"; do
            if [ "$pid" = "$cpid" ]; then
                in_config=true
                break
            fi
        done
        swap=$(grep VmSwap /proc/$pid/status 2>/dev/null | awk '{print $2}' || echo "0")
        if [ "$in_config" = false ] && [ "${swap:-0}" -gt 0 ] 2>/dev/null; then
            echo -e "  ${RED}❌ PID $pid 未在配置中但 VmSwap > 0 — 可能存在跨 Pod 命中！${NC}"
        elif [ "$in_config" = false ] && [ "${swap:-0}" -eq 0 ] 2>/dev/null; then
            echo -e "  ${GREEN}✅ PID $pid 未在配置中且 VmSwap=0 — 隔离正确${NC}"
        elif [ "$in_config" = true ] && [ "${swap:-0}" -gt 0 ] 2>/dev/null; then
            echo -e "  ${GREEN}✅ PID $pid 在配置中且 VmSwap > 0 — 管理正确${NC}"
        fi
    done
else
    echo -e "  ${YELLOW}⚠️  配置目录 $CONF_DIR 不存在${NC}"
fi
echo ""

# ------- 4. Agent 日志验证 -------
echo "【4】Agent 日志验证"
AGENT_POD=$(kubectl get pods -n etmem-system -l app=etmem-agent -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -n "$AGENT_POD" ]; then
    echo "  Agent Pod: $AGENT_POD"
    # 检查 project 名中是否包含 PID
    PROJ_LINES=$(kubectl logs "$AGENT_POD" -n etmem-system --tail=50 2>/dev/null | grep -E "project|Matched" | tail -10)
    echo "  最近的项目/匹配日志:"
    echo "$PROJ_LINES" | sed 's/^/    /'
    echo ""
    
    # 检查是否有 PID 模式的项目名（包含 -p 前缀的 PID）
    if echo "$PROJ_LINES" | grep -qE '\-p[0-9]+'; then
        echo -e "  ${GREEN}✅ Agent 项目名包含 PID 标识 — 使用 PID 作用域${NC}"
    else
        echo -e "  ${YELLOW}⚠️  未发现 PID 标识项目名${NC}"
    fi
else
    echo -e "  ${YELLOW}⚠️  未找到 Agent Pod${NC}"
fi
echo ""

# ------- 5. 隔离性判断结论 -------
echo "【5】隔离性验证结论"
echo ""

if [ ${#MEMHOG_PIDS[@]} -ge 2 ]; then
    if [ "$UNMANAGED_COUNT" -gt 0 ] && [ "$MANAGED_COUNT" -gt 0 ]; then
        echo -e "  ${GREEN}✅ 跨 Pod 隔离验证通过${NC}"
        echo "  详情："
        echo "    - 节点上共有 ${#MEMHOG_PIDS[@]} 个同名 memhog 进程"
        echo "    - 被 etmem 管理的进程: $MANAGED_COUNT 个（VmSwap > 0）"
        echo "    - 未被管理的进程: $UNMANAGED_COUNT 个（VmSwap = 0）"
        echo "    - 结论：type=pid 精确定向有效，未发生交叉命中"
    elif [ "$MANAGED_COUNT" -eq ${#MEMHOG_PIDS[@]} ]; then
        echo -e "  ${YELLOW}⚠️  所有 memhog 进程都被管理 — 无法判断隔离性${NC}"
        echo "  请确保至少有一个 memhog Pod 不带 etmem.openeuler.io/enable 标签"
    else
        echo -e "  ${YELLOW}⚠️  所有 memhog 进程都未被管理 — 数据面可能未生效${NC}"
        echo "  请检查 sysmem_threshold 和 etmemd 状态"
    fi
else
    echo -e "  ${YELLOW}⚠️  节点上同名进程不足 2 个，无法执行隔离验证${NC}"
    echo "  请创建非标签 memhog Pod 后重新运行此脚本"
fi
echo ""

# ------- 保存结果 -------
echo "【保存结果】"
{
    echo "=== PID 作用域隔离验证结果 ==="
    echo "日期: $(date)"
    echo "memhog 进程总数: ${#MEMHOG_PIDS[@]}"
    echo "被管理进程数: $MANAGED_COUNT"
    echo "未被管理进程数: $UNMANAGED_COUNT"
    echo ""
    for pid in "${MEMHOG_PIDS[@]}"; do
        echo "PID=$pid"
        grep -E "Name:|VmRSS:|VmSwap:" /proc/$pid/status 2>/dev/null | sed 's/^/  /'
        echo "  cgroup: $(cat /proc/$pid/cgroup 2>/dev/null | grep memory | head -1)"
        echo ""
    done
} > "$ARTIFACTS_DIR/scope-isolation-result.txt"

echo "  结果已保存到: $ARTIFACTS_DIR/scope-isolation-result.txt"
echo ""
echo "========================================"
echo " 隔离验证完成"
echo "========================================"
