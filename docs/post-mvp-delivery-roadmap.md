# etmem-operator 正式版本交付与演进路线图

## 1. Helm 交付标准

### 1.1 Helm 包内容清单

**标准 Helm Chart 结构**：
```
etmem-operator-0.2.0.tgz
├── Chart.yaml              # Chart 元数据和版本信息
├── values.yaml             # 默认配置参数
├── README.md               # Chart 使用说明
├── crds/                   # CRD 定义文件
│   ├── etmem.openeuler.io_etmempolicies.yaml
│   └── etmem.openeuler.io_etmemnodestates.yaml
└── templates/              # Kubernetes 资源模板
    ├── _helpers.tpl        # 模板辅助函数
    ├── rbac.yaml           # RBAC 权限定义
    ├── serviceaccount.yaml # 服务账号
    ├── operator-deployment.yaml  # Operator 部署
    ├── agent-daemonset.yaml     # Agent DaemonSet
    └── NOTES.txt           # 安装后提示信息
```

**Chart.yaml 标准配置**：
```yaml
apiVersion: v2
name: etmem-operator
description: Kubernetes Operator for openEuler etmem memory tiering
type: application
version: 0.2.0          # Chart 版本
appVersion: "v0.2.0"    # 应用版本
home: https://github.com/openeuler-mirror/etmem-operator
sources:
  - https://github.com/openeuler-mirror/etmem-operator
maintainers:
  - name: openEuler etmem team
    email: etmem@openeuler.org
keywords:
  - memory
  - tiering
  - swap
  - openeuler
  - etmem
annotations:
  operator.openeuler.io/certified: "true"
  operator.openeuler.io/support: "community"
```

### 1.2 values.yaml 参数说明

**核心配置参数**：
```yaml
# Operator 镜像配置
image:
  repository: etmem-operator
  tag: v0.2.0
  pullPolicy: IfNotPresent

# Agent 镜像配置  
agent:
  image:
    repository: etmem-agent
    tag: v0.2.0
    pullPolicy: IfNotPresent

# 资源限制配置
resources:
  operator:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 256Mi
  agent:
    requests:
      cpu: 50m  
      memory: 64Mi
    limits:
      cpu: 500m
      memory: 512Mi

# 节点选择器（可选）
nodeSelector: {}

# 容忍度配置（可选）
tolerations: []

# 亲和性配置（可选）
affinity: {}

# 安全上下文
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  fsGroup: 1000

# 镜像拉取密钥（可选）
imagePullSecrets: []

# 服务账号配置
serviceAccount:
  create: true
  name: ""
  annotations: {}
```

**高级配置参数**：
```yaml
# 默认 Profile 配置
defaultProfile: aggressive

# Agent 配置
agent:
  # etmemd socket 路径
  etmemdSocketPath: /var/run/etmemd.sock
  
  # 健康检查配置
  livenessProbe:
    enabled: true
    initialDelaySeconds: 30
    periodSeconds: 10
    
  # 日志级别
  logLevel: info
  
  # 节点标签选择器
  nodeSelector:
    etmem.openeuler.io/enabled: "true"

# Operator 配置
operator:
  # 并发 reconcile 数量
  maxConcurrentReconciles: 1
  
  # 健康检查端口
  healthPort: 8081
  
  # 指标端口
  metricsPort: 8080
  
  # Webhook 端口（预留）
  webhookPort: 9443
```

### 1.3 CRD 管理策略

**CRD 安装策略**：
- **安装时机**：Helm 安装前预先安装（helm install --create-namespace）
- **升级策略**：保持向前兼容，新增字段使用 optional
- **删除策略**：Helm uninstall 时不自动删除 CRD（防止数据丢失）

**CRD 版本管理**：
```yaml
# EtmemPolicy CRD 版本控制
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: etmempolicies.etmem.openeuler.io
spec:
  versions:
  - name: v1alpha1
    served: true
    storage: true    # 当前存储版本
  - name: v1beta1   # 预留未来版本
    served: false
    storage: false
```

**CRD 独立管理命令**：
```bash
# 手工安装 CRD
kubectl apply -f crds/

# 验证 CRD 安装
kubectl get crd | grep etmem

# 升级 CRD（需要人工确认）
kubectl apply -f crds/ --validate=false

# 清理 CRD（谨慎操作）
kubectl delete -f crds/
```

## 2. 镜像交付

### 2.1 Operator 镜像规范

**镜像命名规范**：
```bash
# 官方镜像命名
registry.openeuler.org/etmem/etmem-operator:v0.2.0

# 多架构支持
registry.openeuler.org/etmem/etmem-operator:v0.2.0-amd64
registry.openeuler.org/etmem/etmem-operator:v0.2.0-arm64

# Latest 标签（指向最新稳定版）
registry.openeuler.org/etmem/etmem-operator:latest
```

**镜像构建信息**：
- **基础镜像**：openeuler/openeuler:23.03-lts（最小化）
- **镜像大小**：< 50MB（压缩后）
- **安全扫描**：无 HIGH/CRITICAL 漏洞
- **SBOM**：包含软件物料清单（Software Bill of Materials）

### 2.2 Agent 镜像规范

**镜像命名规范**：
```bash
# 官方镜像命名  
registry.openeuler.org/etmem/etmem-agent:v0.2.0

# 多架构支持
registry.openeuler.org/etmem/etmem-agent:v0.2.0-amd64
registry.openeuler.org/etmem/etmem-agent:v0.2.0-arm64
```

**特殊要求**：
- **特权模式**：需要 privileged 权限访问宿主机 cgroup
- **主机网络**：使用 hostNetwork 访问 etmemd socket
- **卷挂载**：挂载 `/sys/fs/cgroup` 和 `/var/run`

### 2.3 tar 包格式交付

**镜像 tar 包命名**：
```bash
etmem-operator-v0.2.0-images.tar.gz
├── etmem-operator-v0.2.0-amd64.tar    # Operator 镜像
├── etmem-agent-v0.2.0-amd64.tar       # Agent 镜像
└── load-images.sh                      # 导入脚本
```

**导入脚本示例**：
```bash
#!/bin/bash
# load-images.sh - 镜像导入脚本

set -e

echo "Loading etmem-operator images..."

# 检查 Docker/Podman 可用性
if command -v docker >/dev/null 2>&1; then
    RUNTIME="docker"
elif command -v podman >/dev/null 2>&1; then
    RUNTIME="podman"  
else
    echo "Error: Neither docker nor podman found"
    exit 1
fi

# 导入镜像
echo "Using container runtime: $RUNTIME"
$RUNTIME load -i etmem-operator-v0.2.0-amd64.tar
$RUNTIME load -i etmem-agent-v0.2.0-amd64.tar

# 验证导入结果
echo "Verifying loaded images..."
$RUNTIME images | grep etmem-operator
$RUNTIME images | grep etmem-agent

echo "Images loaded successfully!"
echo "Next steps:"
echo "1. helm install etmem-operator ./etmem-operator-0.2.0.tgz"
echo "2. kubectl get pods -n etmem-system"
```

### 2.4 release.sh 使用说明

**完整发布流程**：
```bash
# 1. 构建所有组件
cd /path/to/etmem-operator
./scripts/release.sh v0.2.0

# 输出目录结构：
# dist/v0.2.0/
# ├── etmem-operator-v0.2.0-amd64.tar      # Operator 镜像 tar
# ├── etmem-agent-v0.2.0-amd64.tar         # Agent 镜像 tar  
# ├── etmem-operator-0.2.0.tgz             # Helm Chart
# ├── checksums.txt                         # SHA256 校验和
# └── release-notes.md                      # 发布说明
```

**release.sh 功能清单**：
1. **代码构建**：`make build-operator build-agent`
2. **镜像构建**：`docker build` 多架构镜像
3. **镜像导出**：`docker save` 生成 tar 包
4. **Helm 打包**：`helm package` 生成 tgz
5. **校验和生成**：`sha256sum` 所有产物
6. **发布说明**：自动生成 CHANGELOG

**使用参数说明**：
```bash
./scripts/release.sh [VERSION] [OPTIONS]

参数：
  VERSION          版本号，如 v0.2.0
  
选项：
  --push           推送镜像到 registry  
  --helm-push      推送 Helm Chart 到 repo
  --dry-run        仅构建，不生成产物
  --arch=ARCH      指定架构（amd64/arm64/all）
  --registry=URL   指定镜像仓库地址

示例：
  ./scripts/release.sh v0.2.0                    # 本地构建
  ./scripts/release.sh v0.2.0 --push            # 构建并推送镜像
  ./scripts/release.sh v0.2.0 --arch=all        # 多架构构建
```

## 3. 正式版本功能路线图

### 3.1 v0.3.0：SlideOverrides 补全

**发布时间**：2024年Q2

**核心特性**：
1. **用户自定义 swapcache 参数**
   ```yaml
   apiVersion: v1
   kind: Pod
   metadata:
     annotations:
       etmem.openeuler.io/profile: "aggressive"
       etmem.openeuler.io/slide-overrides: |
         swapcache_high_wmark: 8
         swapcache_low_wmark: 4
         max_threads: 4
   ```

2. **Agent liveness probe**
   ```yaml
   livenessProbe:
     exec:
       command:
       - /bin/etmem-agent
       - --health-check
     initialDelaySeconds: 30
     periodSeconds: 10
   ```

3. **增强监控指标**
   - Prometheus 指标暴露（`:8080/metrics`）
   - Grafana 仪表盘模板
   - 告警规则模板

**向前兼容**：v0.2.0 API 完全兼容

### 3.2 v0.4.0：性能优化与可观测性

**发布时间**：2024年Q3

**核心特性**：
1. **Agent informer 节点过滤**
   ```go
   // 只监听本节点相关的资源，减少 API 调用
   func (a *Agent) watchNodeSpecificResources() error {
       return ctrl.NewControllerManagedBy(a.Manager).
           For(&corev1.Pod{}).
           WithOptions(controller.Options{
               // 只监听调度到本节点的 Pod
               Namespace: metav1.NamespaceAll,
               FieldSelector: fields.SelectorFromSet(fields.Set{
                   "spec.nodeName": a.NodeName,
               }),
           }).Complete(a)
   }
   ```

2. **E2E 自动化测试框架**
   ```bash
   # 端到端测试套件
   make test-e2e
   
   # 测试覆盖场景：
   # - 部署验证
   # - 功能验证（5 个核心场景）
   # - 性能基线测试
   # - 异常恢复测试
   # - 升级兼容性测试
   ```

3. **配置热重载**
   - Operator 配置文件变更自动重载
   - Agent 参数动态调整（无需重启 Pod）

### 3.3 v1.0.0：生产级完备性

**发布时间**：2024年Q4

**核心特性**：
1. **cgroup v2 支持**
   ```go
   // 自动检测 cgroup 版本并适配
   func (a *Agent) detectCgroupVersion() (CgroupVersion, error) {
       if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
           return CgroupV2, nil
       }
       return CgroupV1, nil
   }
   ```

2. **多租户安全加固**
   - namespace 级别的资源隔离
   - Pod Security Standards 兼容
   - RBAC 最小权限原则
   - 审计日志完善

3. **环境兼容性矩阵验证**
   - 支持 openEuler 22.03/23.03/24.03
   - 支持 FusionOS 22/23  
   - 支持 Kubernetes 1.26-1.30
   - 支持 containerd/CRI-O/Docker

### 3.4 v1.1.0：多容器运行时支持

**发布时间**：2025年Q1  

**核心特性**：
1. **CRI-O 支持**
   ```go
   // 容器运行时适配器
   type ContainerRuntimeAdapter interface {
       GetContainerPIDs(podUID, containerID string) ([]int, error)
       GetCgroupPath(podUID, containerID string) (string, error)
   }
   
   type CRIOAdapter struct{}
   type ContainerdAdapter struct{}
   type DockerAdapter struct{}
   ```

2. **Docker 支持**（兼容性考虑）
   - Docker cgroup 路径适配
   - Docker API 集成

3. **运行时自动检测**
   ```go
   // 自动检测节点容器运行时
   func DetectContainerRuntime() (ContainerRuntime, error) {
       if isContainerdRunning() {
           return ContainerdRuntime, nil
       }
       if isCRIORunning() {
           return CRIORuntime, nil  
       }
       return UnknownRuntime, fmt.Errorf("unsupported runtime")
   }
   ```

### 3.5 v1.2.0：动态配置与智能调度

**发布时间**：2025年Q2

**核心特性**：
1. **动态 Profile 配置**
   ```yaml
   apiVersion: etmem.openeuler.io/v1alpha1
   kind: EtmemProfile
   metadata:
     name: custom-aggressive
   spec:
     slideConfig:
       loop: 1
       interval: 1
       sleep: 1  
       sysMem: 25
       swapcache_high_wmark: 8
       swapcache_low_wmark: 4
   ```

2. **智能节点调度**
   - 基于节点 swap 容量的 Pod 调度优化
   - 内存压力感知的 etmem 启停策略

3. **AI 驱动参数优化**
   - 基于历史数据的参数自动调优
   - 工作负载模式识别和适配

## 4. 环境兼容性矩阵

### 4.1 操作系统支持矩阵

| 操作系统 | 版本 | v0.2.0 | v0.3.0 | v1.0.0 | 备注 |
|----------|------|--------|--------|--------|------|
| openEuler | 22.03 LTS | ⚠️ | ✅ | ✅ | 需内核 >= 5.10 |
| openEuler | 23.03 | ✅ | ✅ | ✅ | 推荐版本 |
| openEuler | 24.03 | 📋 | ✅ | ✅ | 计划支持 |
| FusionOS | 22 | ❌ | ⚠️ | ✅ | 内核兼容性待验证 |
| FusionOS | 23 | ✅ | ✅ | ✅ | 已验证支持 |
| Ubuntu | 20.04+ | ❌ | ❌ | 📋 | 社区需求驱动 |
| CentOS | Stream 9 | ❌ | ❌ | 📋 | 需要 etmem 内核模块 |

**图例**：✅ 支持 | ⚠️ 实验性支持 | 📋 计划支持 | ❌ 不支持

### 4.2 容器运行时支持矩阵

| 容器运行时 | 版本 | cgroup v1 | cgroup v2 | v0.2.0 | v1.0.0 |
|------------|------|-----------|-----------|--------|--------|
| containerd | 1.6+ | ✅ | 📋 | ✅ | ✅ |
| containerd | 1.7+ | ✅ | ✅ | ✅ | ✅ |
| CRI-O | 1.26+ | ⚠️ | 📋 | ❌ | ✅ |
| Docker | 20.10+ | ⚠️ | ❌ | ❌ | ✅ |
| Docker | 24.0+ | ✅ | 📋 | ❌ | ✅ |

### 4.3 Kubernetes 版本支持矩阵

| Kubernetes | API 版本 | v0.2.0 | v0.3.0 | v1.0.0 | 生命周期 |
|------------|----------|--------|--------|--------|----------|
| 1.25 | v1.25.16 | ⚠️ | ❌ | ❌ | EOL 2024-10 |
| 1.26 | v1.26.15 | ✅ | ✅ | ✅ | 推荐版本 |
| 1.27 | v1.27.12 | ✅ | ✅ | ✅ | 稳定支持 |
| 1.28 | v1.28.8+ | ✅ | ✅ | ✅ | 稳定支持 |
| 1.29 | v1.29.3+ | 📋 | ✅ | ✅ | 计划支持 |
| 1.30 | v1.30.0+ | ❌ | 📋 | ✅ | 未来支持 |

### 4.4 依赖组件要求

**必需组件**：
- **etmemd**：2.0+ 版本，支持抽象 Unix socket
- **内核模块**：etmem_scan + etmem_swap，内核 >= 5.10
- **swap 设备**：建议容量 >= 物理内存的 50%
- **cgroup**：memory 子系统已挂载

**可选组件**：
- **Prometheus**：v2.35+ 用于指标采集
- **Grafana**：v9.0+ 用于可视化监控  
- **Helm**：v3.8+ 用于部署管理

### 4.5 硬件资源建议

**最小配置**：
- **CPU**：2 核
- **内存**：4GB （推荐 8GB）
- **存储**：10GB 可用空间
- **swap**：2GB （推荐与内存同等大小）

**生产环境推荐**：
- **CPU**：4 核以上
- **内存**：16GB 以上  
- **存储**：50GB 以上 SSD
- **swap**：内存容量的 100-200%
- **网络**：千兆以太网

### 4.6 安全要求

**权限要求**：
- **Operator**：cluster-admin 或自定义 RBAC
- **Agent**：privileged 权限（访问宿主机 cgroup）
- **用户**：pods 资源的 get/list/patch 权限

**网络要求**：  
- **集群内通信**：Kubernetes API Server 访问
- **节点通信**：etmemd Unix socket 访问
- **外部访问**：镜像拉取（可选，支持离线部署）

**安全加固**：
- Pod Security Standards：Privileged（Agent 需要）
- SELinux/AppArmor：兼容性验证
- 网络策略：最小权限网络访问控制