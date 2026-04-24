# etmem-operator v0.2.0 使用与验证指导手册

## 1. 环境前提条件

### 1.1 操作系统与内核要求

**检查操作系统版本**：
```bash
cat /etc/os-release
# 期望输出：FusionOS 23 或 openEuler 22.03/23.03
```

**检查内核版本**：
```bash
uname -r
# 期望输出：5.10.0-278 或更高版本
```

**检查 etmem 内核模块**：
```bash
lsmod | grep etmem
# 期望输出：etmem_scan, etmem_swap 等模块
```

### 1.2 etmemd 服务状态

**检查 etmemd 服务运行**：
```bash
systemctl status etmemd
# 期望输出：Active: active (running)
```

**检查 etmemd Unix socket**：
```bash
ss -x | grep etmemd
# 期望输出：/var/run/etmemd.sock 抽象 socket
```

**测试 etmemd 连接**：
```bash
echo '{"cmd":"help"}' | nc -U /var/run/etmemd.sock
# 期望输出：etmemd 命令帮助信息
```

### 1.3 Swap 配置

**检查 swap 设备**：
```bash
swapon --show
# 期望输出：至少一个 swap 设备，如 /dev/mapper/vg-swap
```

**检查 swap 空间**：
```bash
free -h
# 期望输出：Swap 行显示总容量 > 0，如 15Gi
```

### 1.4 cgroup 版本确认

**检查 cgroup 版本**：
```bash
stat -fc %T /sys/fs/cgroup/
# 期望输出：tmpfs (表示 cgroup v1)
# 如果输出 cgroup2fs 则为 v2，暂不支持
```

**检查 memory cgroup 挂载**：
```bash
mount | grep memory
# 期望输出：cgroup on /sys/fs/cgroup/memory type cgroup (rw,memory)
```

### 1.5 容器运行时

**检查 containerd 版本**：
```bash
containerd --version
# 期望输出：containerd 1.6.22 或更高
```

**检查容器运行时配置**：
```bash
kubectl get nodes -o wide
# 期望输出：Container Runtime 列显示 containerd://1.6.22
```

### 1.6 Kubernetes 集群

**检查集群版本**：
```bash
kubectl version --short
# 期望输出：Client/Server Version v1.26.15 或兼容版本
```

**检查节点就绪**：
```bash
kubectl get nodes
# 期望输出：所有节点 STATUS 为 Ready
```

## 2. 部署步骤

### 2.1 部署 CRD 资源

```bash
cd /home/ygz/work/etmem-workspace/etmem-operator

# 应用 EtmemPolicy CRD
kubectl apply -f deploy/helm/crds/etmem.openeuler.io_etmempolicies.yaml
# 期望输出：customresourcedefinition.apiextensions.k8s.io/etmempolicies.etmem.openeuler.io created

# 应用 EtmemNodeState CRD
kubectl apply -f deploy/helm/crds/etmem.openeuler.io_etmemnodestates.yaml
# 期望输出：customresourcedefinition.apiextensions.k8s.io/etmemnodestates.etmem.openeuler.io created

# 验证 CRD 安装
kubectl get crd | grep etmem
# 期望输出：
# etmemnodestates.etmem.openeuler.io    <date>
# etmempolicies.etmem.openeuler.io     <date>
```

### 2.2 Helm 安装 Operator

```bash
# 从源码目录安装
helm install etmem-operator ./deploy/helm/ \
  --namespace etmem-system \
  --create-namespace \
  --set image.repository=etmem-operator \
  --set image.tag=v0.2.0 \
  --set agent.image.repository=etmem-agent \
  --set agent.image.tag=v0.2.0

# 期望输出：
# NAME: etmem-operator
# NAMESPACE: etmem-system
# STATUS: deployed
```

> 请在 etmem-operator 仓库根目录执行该命令；如果当前目录不是仓库根目录，请将 `./deploy/helm/` 改成完整路径。

### 2.3 验证部署状态

**检查 Operator Pod**：
```bash
kubectl get pods -n etmem-system
# 期望输出：
# NAME                               READY   STATUS    RESTARTS   AGE
# etmem-operator-xxx                 1/1     Running   0          30s
# etmem-agent-xxx                    1/1     Running   0          30s
```

**检查 Agent DaemonSet**：
```bash
kubectl get ds -n etmem-system
# 期望输出：
# NAME          DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
# etmem-agent   1         1         1       1            1           <none>          30s
```

**检查日志无错误**：
```bash
kubectl logs -n etmem-system deployment/etmem-operator
# 期望输出：包含 "Starting manager" 且无 ERROR 日志

kubectl logs -n etmem-system ds/etmem-agent
# 期望输出：包含 "Agent started successfully" 且无 ERROR 日志
```

## 3. 使用方法

### 3.1 启用 etmem 内存分层

**为现有 Pod 添加标签**：
```bash
kubectl label pod <pod-name> etmem.openeuler.io/enable=true
# 期望输出：pod/<pod-name> labeled
```

**创建新 Pod 时直接启用**：
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-app
  labels:
    etmem.openeuler.io/enable: "true"
spec:
  containers:
  - name: app
    image: nginx:1.20
    resources:
      requests:
        memory: 100Mi
      limits:
        memory: 500Mi
```

### 3.2 选择性能档位

**添加 profile annotation**：
```bash
# 保守档位（最温和）
kubectl annotate pod <pod-name> etmem.openeuler.io/profile=conservative

# 中等档位
kubectl annotate pod <pod-name> etmem.openeuler.io/profile=moderate

# 激进档位（默认）
kubectl annotate pod <pod-name> etmem.openeuler.io/profile=aggressive

# 极端档位（最激进）
kubectl annotate pod <pod-name> etmem.openeuler.io/profile=extreme
```

### 3.3 禁用 etmem 内存分层

**移除 enable 标签**：
```bash
kubectl label pod <pod-name> etmem.openeuler.io/enable-
# 期望输出：pod/<pod-name> labeled
```

**移除 profile annotation**：
```bash
kubectl annotate pod <pod-name> etmem.openeuler.io/profile-
# 期望输出：pod/<pod-name> annotated
```

## 4. 验证场景

### 4.1 场景 A：启用标签触发 swap

**步骤 1：创建测试 Pod**
```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: memory-test-app
  namespace: default
spec:
  containers:
  - name: app
    image: busybox:1.35
    command: ["sh", "-c"]
    args: ["dd if=/dev/zero of=/tmp/bigfile bs=1M count=100 && sleep 3600"]
    resources:
      requests:
        memory: 50Mi
      limits:
        memory: 200Mi
EOF

# 期望输出：pod/memory-test-app created
```

**步骤 2：等待 Pod 运行并消耗内存**
```bash
kubectl wait --for=condition=Ready pod/memory-test-app --timeout=60s
# 期望输出：pod/memory-test-app condition met

# 检查初始内存状态（应该无 swap）
kubectl exec memory-test-app -- cat /proc/self/status | grep VmSwap
# 期望输出：VmSwap:        0 kB
```

**步骤 3：启用 etmem**
```bash
kubectl label pod memory-test-app etmem.openeuler.io/enable=true
# 期望输出：pod/memory-test-app labeled
```

**步骤 4：检查自动策略创建**
```bash
kubectl get etmempolicy -n default
# 期望输出：
# NAME         AGE
# etmem-auto   10s

kubectl get etmempolicy etmem-auto -n default -o jsonpath='{.spec.engine.profile}'
# 期望输出：aggressive
```

**步骤 5：等待 swap 生效**
```bash
# 等待 30 秒让 etmem 扫描生效
sleep 30

# 检查进程 swap 使用量
kubectl exec memory-test-app -- cat /proc/self/status | grep VmSwap
# 期望输出：VmSwap:     xxxx kB （大于 0）
```

**成功标准**：VmSwap > 0，说明内存已被换出到 swap

### 4.2 场景 B：移除标签停止 swap

**步骤 1：移除 etmem 标签**
```bash
kubectl label pod memory-test-app etmem.openeuler.io/enable-
# 期望输出：pod/memory-test-app labeled
```

**步骤 2：检查自动策略删除**
```bash
kubectl get etmempolicy -n default
# 期望输出：No resources found in default namespace.
```

**步骤 3：检查 etmem 任务停止**
```bash
kubectl get etmemnodestate
# 期望输出：RunningTasks 应为空或不包含 default namespace 的任务
```

**成功标准**：etmem-auto 策略被自动删除，etmem 停止对该 Pod 的处理

### 4.3 场景 C：删除 Pod 清理策略

**步骤 1：重新启用 etmem**
```bash
kubectl label pod memory-test-app etmem.openeuler.io/enable=true
```

**步骤 2：确认策略存在**
```bash
kubectl get etmempolicy etmem-auto -n default
# 期望输出：显示策略详情
```

**步骤 3：删除 Pod**
```bash
kubectl delete pod memory-test-app
# 期望输出：pod "memory-test-app" deleted
```

**步骤 4：检查策略自动清理**
```bash
# 等待 10 秒让 Operator 检测到 Pod 删除
sleep 10

kubectl get etmempolicy -n default
# 期望输出：No resources found in default namespace.
```

**成功标准**：Pod 删除后，etmem-auto 策略自动清理

### 4.4 场景 D：Pod 重建自动恢复

**步骤 1：创建带 etmem 标签的 Deployment**
```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: etmem-test-deploy
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: etmem-test
  template:
    metadata:
      labels:
        app: etmem-test
        etmem.openeuler.io/enable: "true"
      annotations:
        etmem.openeuler.io/profile: "moderate"
    spec:
      containers:
      - name: app
        image: nginx:1.20
        resources:
          requests:
            memory: 50Mi
          limits:
            memory: 200Mi
EOF

# 期望输出：deployment.apps/etmem-test-deploy created
```

**步骤 2：确认初始状态**
```bash
kubectl get pods -l app=etmem-test
# 期望输出：显示一个 Running 状态的 Pod

kubectl get etmempolicy etmem-auto -n default -o jsonpath='{.spec.engine.profile}'
# 期望输出：moderate（或创建 Deployment 时 Pod annotation 中指定的 profile）
```

**步骤 3：触发 Pod 重建**
```bash
kubectl rollout restart deployment/etmem-test-deploy
# 期望输出：deployment.apps/etmem-test-deploy restarted
```

**步骤 4：检查新 Pod 继承配置**
```bash
# 等待新 Pod 启动
kubectl rollout status deployment/etmem-test-deploy
# 期望输出：deployment "etmem-test-deploy" successfully rolled out

# 检查新 Pod 标签
kubectl get pods -l app=etmem-test -o jsonpath='{.items[0].metadata.labels.etmem\.openeuler\.io/enable}'
# 期望输出：true

# 检查策略仍存在且配置正确
kubectl get etmempolicy etmem-auto -n default -o jsonpath='{.spec.engine.profile}'
# 期望输出：显示当前最激进的 profile（如 moderate）
```

**成功标准**：新 Pod 自动继承 etmem 配置，策略持续有效

### 4.5 场景 E：Profile annotation 生效

**步骤 1：创建默认 aggressive 的 Pod**
```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: profile-test-pod
  namespace: default
  labels:
    etmem.openeuler.io/enable: "true"
spec:
  containers:
  - name: app
    image: busybox:1.35
    command: ["sleep", "3600"]
EOF
```

**步骤 2：检查默认 aggressive 配置**
```bash
kubectl get etmempolicy etmem-auto -n default -o jsonpath='{.spec.engine.profile}'
# 期望输出：aggressive
```

**步骤 3：修改为 extreme 档位**
```bash
kubectl annotate pod profile-test-pod etmem.openeuler.io/profile=extreme
# 期望输出：pod/profile-test-pod annotated
```

**步骤 4：检查配置更新**
```bash
# 等待 5 秒让 Operator 处理变更
sleep 5

kubectl get etmempolicy etmem-auto -n default -o jsonpath='{.spec.engine.profile}'
# 期望输出：extreme

# 验证 Agent 生成的 etmem 配置文件中 swapcache 参数（在节点上执行）
kubectl -n etmem-system exec ds/etmem-agent -- cat /var/run/etmem/configs/*.conf 2>/dev/null | grep swapcache
# 期望输出：swapcache_high_wmark=2 和 swapcache_low_wmark=1（extreme 档位特征值）
```

**步骤 5：清理测试资源**
```bash
kubectl delete pod profile-test-pod
kubectl delete deployment etmem-test-deploy
```

**成功标准**：profile annotation 变更后，etmem-auto 策略配置实时更新到对应档位参数

## 5. 监控指标说明

### 5.1 进程级内存监控

**查看单个进程 swap 使用**：
```bash
# 方法1：通过 Pod 内查看
kubectl exec <pod-name> -- cat /proc/self/status | grep -E "(VmRSS|VmSwap)"
# 输出示例：
# VmRSS:    45320 kB  （物理内存）
# VmSwap:   12580 kB  （swap 使用量）

# 方法2：在节点上查看容器进程
ps aux | grep <container-process>
cat /proc/<PID>/status | grep -E "(VmRSS|VmSwap)"
```

### 5.2 系统级内存监控

**系统总体内存状态**：
```bash
free -h
# 输出示例：
#               total        used        free      shared  buff/cache   available
# Mem:           15Gi        8.2Gi       1.1Gi       264Mi       6.0Gi       6.4Gi
# Swap:          15Gi        856Mi        14Gi
```

**详细内存统计**：
```bash
cat /proc/meminfo | grep -E "(MemTotal|MemAvailable|SwapTotal|SwapFree|SwapCached)"
# 输出示例：
# MemTotal:       16384000 kB
# MemAvailable:    6553600 kB  （可用内存，关键指标）
# SwapTotal:      15728640 kB
# SwapFree:       14856192 kB
# SwapCached:       872448 kB  （swapcache 使用量）
```

### 5.3 集群级资源监控

**查看 EtmemNodeState**：
```bash
kubectl get etmemnodestate -o wide
# 输出示例：
# NAME   NODE   RUNNING-TASKS   LAST-UPDATE
# cp0    cp0    2               2024-01-15T10:30:00Z

kubectl describe etmemnodestate <node-name>
# 输出包含：RunningTasks 详情，BootstrapTime，LastUpdate
```

**查看 EtmemPolicy**：
```bash
kubectl get etmempolicy -A
# 输出示例：
# NAMESPACE   NAME         AGE
# default     etmem-auto   5m
# test        etmem-auto   2m

kubectl get etmempolicy etmem-auto -n <namespace> -o yaml
# 输出包含：完整的 slideConfig 配置参数
```

### 5.4 Agent 操作日志

**查看 Agent 执行日志**：
```bash
kubectl logs -n etmem-system ds/etmem-agent -f
# 关键日志示例：
# 2024-01-15T10:30:00Z INFO Agent started successfully
# 2024-01-15T10:30:30Z INFO Policy updated {"namespace": "default", "policy": "etmem-auto"}
# 2024-01-15T10:31:00Z INFO Task started {"namespace": "default", "pod": "memory-test-app"}
# 2024-01-15T10:31:30Z INFO Swap progress {"pid": 12345, "swapped": "12580kB"}
```

**查看 Operator 控制日志**：
```bash
kubectl logs -n etmem-system deployment/etmem-operator -f
# 关键日志示例：
# 2024-01-15T10:30:00Z INFO Starting manager
# 2024-01-15T10:30:30Z INFO Pod reconcile {"namespace": "default", "pod": "memory-test-app"}
# 2024-01-15T10:30:31Z INFO Policy created {"namespace": "default", "policy": "etmem-auto"}
```

## 6. 排查指南

### 6.1 常见问题排查

**问题1：Pod 标签后无 swap 发生**

排查步骤：
```bash
# 1. 检查标签是否正确
kubectl get pod <pod-name> -o yaml | grep -A2 -B2 etmem

# 2. 检查策略是否创建
kubectl get etmempolicy -n <namespace>

# 3. 检查 Agent 是否运行
kubectl get pods -n etmem-system

# 4. 检查 Agent 日志
kubectl logs -n etmem-system ds/etmem-agent | grep ERROR

# 5. 检查进程内存使用是否达到阈值
kubectl exec <pod-name> -- cat /proc/self/status | grep VmRSS
```

**问题2：策略创建失败**

排查步骤：
```bash
# 1. 检查 Operator 权限
kubectl auth can-i create etmempolicies --as=system:serviceaccount:etmem-system:etmem-operator

# 2. 检查 CRD 是否安装
kubectl get crd etmempolicies.etmem.openeuler.io

# 3. 检查 Operator 日志
kubectl logs -n etmem-system deployment/etmem-operator | grep -E "(ERROR|WARN)"
```

**问题3：Agent 无法连接 etmemd**

排查步骤：
```bash
# 1. 检查 etmemd 服务状态
systemctl status etmemd

# 2. 检查 socket 权限
ls -la /var/run/etmemd.sock

# 3. 测试 socket 连接
echo '{"cmd":"help"}' | nc -U /var/run/etmemd.sock

# 4. 检查 Agent 是否有权限访问 socket
kubectl exec -n etmem-system ds/etmem-agent -- ls -la /var/run/etmemd.sock
```

### 6.2 环境验证清单

使用前请确认以下检查点：

- [ ] FusionOS/openEuler 系统，内核 5.10+
- [ ] etmemd 服务运行中，socket 可访问
- [ ] swap 设备已配置，空间 > 1GB
- [ ] cgroup v1 已挂载 memory 子系统
- [ ] containerd 1.6+ 运行正常
- [ ] Kubernetes 1.26+ 集群可用
- [ ] etmem CRD 已安装
- [ ] etmem-operator Pod 运行正常
- [ ] etmem-agent DaemonSet 覆盖所有节点
- [ ] RBAC 权限配置正确

### 6.3 紧急恢复步骤

**如果 etmem 导致系统问题**：

1. **立即禁用所有 Pod**：
```bash
kubectl get pods --all-namespaces -l etmem.openeuler.io/enable=true \
  --no-headers | awk '{print "kubectl label pod "$2" etmem.openeuler.io/enable- -n "$1}' | bash
```

2. **删除所有自动策略**：
```bash
kubectl get etmempolicy --all-namespaces -l etmem.openeuler.io/auto-generated=true \
  --no-headers | awk '{print "kubectl delete etmempolicy "$2" -n "$1}' | bash
```

3. **停止 Operator**：
```bash
kubectl scale deployment etmem-operator -n etmem-system --replicas=0
```

4. **停止所有 Agent**：
```bash
kubectl scale daemonset etmem-agent -n etmem-system --replicas=0
```

5. **清理 swap（慎重）**：
```bash
# 仅在确认安全的情况下执行
swapoff -a && swapon -a
```
