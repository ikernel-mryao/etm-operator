# etmem-operator

基于 openEuler etmem 的 Kubernetes 内存分级管理方案。

## 架构概览

```
┌─────────────────────────────────────────────────┐
│                  Kubernetes 集群                  │
│                                                   │
│  ┌─────────────┐     ┌──────────────────────┐   │
│  │  Operator    │     │   Node (DaemonSet)    │   │
│  │  (Deployment)│     │  ┌──────────┐        │   │
│  │             │     │  │  Agent    │        │   │
│  │  - 状态聚合  │     │  │          │        │   │
│  │  - Policy    │     │  │ - 自治推导 │        │   │
│  │    校验      │     │  │ - PID 解析 │        │   │
│  └──────┬──────┘     │  │ - 任务管理 │        │   │
│         │            │  │ - 熔断保护 │        │   │
│         │ Status     │  └─────┬────┘        │   │
│         │ 聚合       │        │ exec CLI     │   │
│         │            │  ┌─────▼────┐        │   │
│         │            │  │ etmemd   │        │   │
│         │            │  │ (systemd)│        │   │
│         │            │  └──────────┘        │   │
│  ┌──────▼──────┐     └──────────────────────┘   │
│  │ EtmemPolicy │                                 │
│  │ (期望状态)   │     ┌──────────────────────┐   │
│  └─────────────┘     │ EtmemNodeState        │   │
│                      │ (观测状态, per-node)    │   │
│                      └──────────────────────┘   │
└─────────────────────────────────────────────────┘
```

## 核心组件

| 组件 | 类型 | 职责 |
|------|------|------|
| Operator | Deployment | 监听 EtmemPolicy，聚合 NodeState 状态到 Policy.Status |
| Agent | DaemonSet | 自治推导本节点任务：匹配 Pod → 解析 PID → 生成配置 → 管理 etmem 任务 |
| etmemd | Host systemd | 宿主机内存分级引擎，独立于 K8s 生命周期 |

## CRD 说明

### EtmemPolicy（namespace 级）
定义内存分级策略的期望状态：
- **selector**: 通过 labelSelector 选择目标 Pod
- **processFilter**: 指定要管理的进程名白名单
- **engine**: 引擎类型（MVP 仅 slide）+ profile 预设 + 参数覆写
- **suspend**: 一键全停开关
- **circuitBreaker**: 熔断阈值配置

### EtmemNodeState（cluster 级，per-node）
报告节点级观测状态：
- 当前管理的任务列表（policy → pod → 进程）
- 节点级指标（管理 Pod 数、swap 字节数、PSI）
- etmemd 连接状态

## Profile 预设

| Profile | 适用场景 | 扫描频率 | 内存阈值 |
|---------|---------|---------|---------|
| conservative | 生产核心业务 | 低（loop=3, interval=5s） | 高（70%） |
| moderate | 通用场景（默认） | 中（loop=1, interval=1s） | 中（50%） |
| aggressive | 批处理/离线任务 | 高（loop=1, sleep=0） | 低（30%） |

## 安全机制

### 熔断器（Circuit Breaker）
- **Pod 级**: 容器 OOMKilled 或 restartCount 超阈值 → 跳过该 Pod
- **节点级**: 内存 PSI avg10 超阈值 → 停止节点所有 etmem 任务
- **不自动恢复**: 需人工介入排查后手动重启

### 回滚
- `spec.suspend: true` → Agent 停止该 Policy 所有任务
- 任务停止后 etmem 自动释放 swap 空间

## 快速开始

### 前置条件
- Kubernetes 1.28+
- 节点已安装 etmemd + etmem CLI + etmem_scan + etmem_swap 内核模块
- etmemd systemd 服务已启动

### 部署
```bash
# 安装 CRD 和组件
helm install etmem-operator ./deploy/helm/ -n etmem-system --create-namespace
```

> 请在仓库根目录执行上述命令。如果您当前目录不是仓库根目录，请改用绝对路径或完整相对路径，例如：
> `helm install etmem-operator /path/to/etmem-operator/deploy/helm -n etmem-system --create-namespace`

### 创建策略
```yaml
apiVersion: etmem.openeuler.io/v1alpha1
kind: EtmemPolicy
metadata:
  name: mysql-memory-tiering
  namespace: default
spec:
  selector:
    matchLabels:
      app: mysql
  processFilter:
    names: ["mysqld"]
  engine:
    type: slide
    profile: moderate
```

### 查看状态
```bash
# 查看策略状态
kubectl get etmempolicies

# 查看节点状态
kubectl get etmemnodestates

# 一键暂停
kubectl patch etmempolicy mysql-memory-tiering -p '{"spec":{"suspend":true}}' --type=merge
```

## 构建

```bash
# 构建二进制
make build

# 运行测试
make test

# 生成 CRD
make manifests

# 构建镜像
make docker-build
```

## 目录结构

```
etmem-operator/
├── api/v1alpha1/          # CRD 类型定义
├── cmd/
│   ├── operator/          # Operator 入口
│   └── agent/             # Agent 入口
├── internal/
│   ├── agent/             # Agent 核心逻辑（reconcile、熔断、PID、NodeState）
│   ├── controller/        # Operator 控制器（状态聚合）
│   ├── engine/            # 引擎抽象 + slide 实现
│   ├── transport/         # etmem CLI 交互抽象
│   └── config/            # 常量和默认配置
├── deploy/helm/           # Helm chart
├── build/                 # Dockerfiles
├── config/crd/bases/      # 生成的 CRD YAML
└── test/e2e/              # E2E 测试骨架
```

## MVP 限制

- 仅支持 slide 引擎（架构已预留多引擎扩展）
- 仅支持 labelSelector 选择 Pod（WorkloadRefs 显式拒绝）
- 仅支持 cgroup v1 cgroupfs 驱动
- 熔断后不自动恢复
- Agent 以固定间隔 reconcile（不按 Policy 粒度调节）
