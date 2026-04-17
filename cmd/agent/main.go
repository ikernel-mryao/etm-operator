// Agent 是 etmem-operator 的节点侧组件，通过 DaemonSet 部署到每个节点。
// 职责：自治推导本节点任务，将匹配的 Pod 转换为 etmem 配置并管理其生命周期。
// 架构定位：Agent 自主决策任务执行，Operator 仅聚合状态，避免中心化瓶颈。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
	"github.com/openeuler/etmem-operator/internal/agent"
	"github.com/openeuler/etmem-operator/internal/config"
	"github.com/openeuler/etmem-operator/internal/engine"
	"github.com/openeuler/etmem-operator/internal/transport"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(etmemv1.AddToScheme(scheme))
}

// isInfraProcess returns true for Kubernetes infrastructure container processes
// (pause, sandbox, etc.) that should not be targeted for memory swap.
func isInfraProcess(name string) bool {
	switch name {
	case "pause", "sandbox":
		return true
	}
	return false
}

func main() {
	var reconcileInterval time.Duration
	var socketName string
	flag.DurationVar(&reconcileInterval, "reconcile-interval", config.DefaultReconcileInterval, "Reconcile loop interval")
	flag.StringVar(&socketName, "socket-name", config.DefaultSocketName, "etmemd socket name")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	logger := ctrl.Log.WithName("agent")

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		logger.Error(nil, "NODE_NAME environment variable is required")
		os.Exit(1)
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		logger.Error(err, "unable to get in-cluster config")
		os.Exit(1)
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		logger.Error(err, "unable to create k8s client")
		os.Exit(1)
	}

	// 启动时一次性获取节点标签，避免每次 reconcile 重复查询。
	// 节点标签变更需重启 Agent pod 才能生效（符合 DaemonSet 滚动更新语义）。
	var node corev1.Node
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: nodeName}, &node); err != nil {
		logger.Error(err, "unable to get node")
		os.Exit(1)
	}
	nodeLabels := node.Labels

	executor := &transport.RealExecutor{}
	tr := transport.NewExecTransport(socketName, executor)
	tm := agent.NewTaskManager(tr, config.EtmemConfigDir)
	pidResolver := agent.NewPIDResolver(config.HostProcPath, config.HostCgroupPath)
	slideEngine := &engine.SlideEngine{}

	// 运行时检测 cgroup 驱动类型：systemd vs cgroupfs
	// 通过探测 /host/sys/fs/cgroup/memory/kubepods.slice 是否存在来判断
	useSystemdCgroup := false
	if _, err := os.Stat(config.HostCgroupPath + "/memory/kubepods.slice"); err == nil {
		useSystemdCgroup = true
		logger.Info("Detected systemd cgroup driver")
	} else {
		logger.Info("Using cgroupfs cgroup driver")
	}

	// C3: Initialize CircuitBreaker and NodeStateWriter
	cb := agent.NewCircuitBreaker(config.DefaultPodRestartThreshold, config.DefaultNodePSIThreshold, "")
	nsWriter := agent.NewNodeStateWriter(k8sClient, nodeName)

	// Bootstrap: restore running task map from NodeState to survive restarts
	var existingNodeState etmemv1.EtmemNodeState
	nsKey := types.NamespacedName{Name: nodeName}
	if err := k8sClient.Get(context.Background(), nsKey, &existingNodeState); err == nil {
		recovered := tm.BootstrapFromNodeState(existingNodeState.Status.Tasks)
		logger.Info("Bootstrapped running tasks from NodeState", "recovered", recovered)
	} else {
		logger.V(1).Info("No existing NodeState found, starting fresh")
	}

	logger.Info("Agent starting", "node", nodeName, "interval", reconcileInterval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		logger.Info("Shutting down, stopping all tasks")
		tm.StopAll(context.Background())
		cancel()
	}()

	// 固定间隔 reconcile 模式：每个节点独立推导任务，无需等待中心化调度。
	// 间隔内发生的 Policy/Pod 变更会在下一轮 reconcile 生效。
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Agent stopped")
			return
		case <-ticker.C:
			start := time.Now()
			if err := agentReconcile(ctx, k8sClient, tm, pidResolver, slideEngine, cb, nsWriter, nodeName, nodeLabels, useSystemdCgroup, logger); err != nil {
				logger.Error(err, "reconcile failed")
				agent.ReconcileErrors.Inc()
			}
			agent.ReconcileDuration.Observe(time.Since(start).Seconds())
		}
	}
}

type desiredTaskInfo struct {
	Request     agent.TaskRequest
	PolicyRef   etmemv1.PolicyReference
	PodName     string
	PodUID      string
	ProcessName string
}

// agentReconcile 执行节点级任务推导的 5 步 reconcile 流程：
// 1. 列出所有 EtmemPolicy
// 2. 列出所有 Pod（客户端侧通过 MatchPodToPolicy 过滤）
// 3. 构建期望任务集合（匹配 Pod → 解析 PID → 每个进程生成独立 project/配置）
// 4. Diff：停止不再需要的任务
// 5. 启动新任务
// 每个应用进程对应一个独立的 etmemd project（etmemd 不支持多 task 共享 project）。
// 最后更新 EtmemNodeState 反映当前节点观测状态。
func agentReconcile(
	ctx context.Context,
	k8sClient client.Client,
	tm *agent.TaskManager,
	pidResolver *agent.PIDResolver,
	slideEngine *engine.SlideEngine,
	cb *agent.CircuitBreaker,
	nsWriter *agent.NodeStateWriter,
	nodeName string,
	nodeLabels map[string]string,
	useSystemdCgroup bool,
	logger logr.Logger,
) error {
	// C3: Check node-level circuit breaker
	if tripped, psi := cb.IsNodeTripped(); tripped {
		logger.Info("Node-level circuit breaker tripped, stopping all tasks", "psi", psi)
		agent.CircuitBreakerTrips.Inc()
		tm.StopAll(ctx)
		return nil
	}

	// 1. List all EtmemPolicies
	var policyList etmemv1.EtmemPolicyList
	if err := k8sClient.List(ctx, &policyList); err != nil {
		return fmt.Errorf("list policies: %w", err)
	}

	// C1: List all pods (client-side filtering via MatchPodToPolicy)
	var podList corev1.PodList
	if err := k8sClient.List(ctx, &podList); err != nil {
		return fmt.Errorf("list pods: %w", err)
	}

	logger.V(1).Info("Reconcile cycle", "policies", len(policyList.Items), "pods", len(podList.Items))

	// 3. Build desired task set
	desiredTasks := make(map[string]desiredTaskInfo)

	for i := range policyList.Items {
		policy := &policyList.Items[i]
		if policy.Spec.Suspend {
			continue
		}
		// S6: Skip policies with WorkloadRefs (not supported in MVP)
		if len(policy.Spec.WorkloadRefs) > 0 {
			continue
		}

		// S3: Default to "moderate" if profile is empty
		profile := policy.Spec.Engine.Profile
		if profile == "" {
			profile = "moderate"
		}
		params, err := engine.GetProfile(profile)
		if err != nil {
			logger.Error(err, "invalid profile", "policy", policy.Name, "profile", profile)
			continue
		}
		params = engine.ApplyOverrides(params, policy.Spec.Engine.Overrides)

		for j := range podList.Items {
			pod := &podList.Items[j]
			if !agent.MatchPodToPolicy(pod, policy, nodeName, nodeLabels) {
				continue
			}

			// C3: Check pod-level circuit breaker
			if cb.IsPodTripped(pod) {
				logger.Info("Pod-level circuit breaker tripped", "pod", pod.Name)
				agent.CircuitBreakerTrips.Inc()
				continue
			}

			// 根据 cgroup 驱动类型选择正确的路径格式
			qosClass := string(pod.Status.QOSClass)
			var cgroupPath string
			if useSystemdCgroup {
				cgroupPath = agent.BuildCgroupRelPathSystemd(string(pod.UID), qosClass)
			} else {
				cgroupPath = agent.BuildCgroupRelPath(string(pod.UID), qosClass)
			}
			var processNames []string
			if policy.Spec.ProcessFilter != nil {
				processNames = policy.Spec.ProcessFilter.Names
			}
			pids, err := pidResolver.ResolvePIDs(cgroupPath, processNames)
			if err != nil {
				logger.V(1).Info("PID resolution failed", "pod", pod.Name, "cgroupPath", cgroupPath, "error", err)
				continue
			}
			if len(pids) == 0 {
				filterDesc := "<all>"
				if policy.Spec.ProcessFilter != nil {
					filterDesc = fmt.Sprintf("%v", policy.Spec.ProcessFilter.Names)
				}
				logger.V(1).Info("No matching PIDs found", "pod", pod.Name, "cgroupPath", cgroupPath, "filter", filterDesc)
				continue
			}
			logger.Info("Matched pod", "pod", pod.Name, "pids", len(pids), "policy", policy.Name)

			// Filter out infrastructure containers (pause/sandbox) and deduplicate.
			seenNames := make(map[string]bool)
			var processes []engine.ProcessTarget
			for _, pid := range pids {
				if isInfraProcess(pid.Name) {
					continue
				}
				if !seenNames[pid.Name] {
					processes = append(processes, engine.ProcessTarget{Name: pid.Name})
					seenNames[pid.Name] = true
				}
			}
			if len(processes) == 0 {
				logger.V(1).Info("No application processes after filtering infrastructure containers", "pod", pod.Name)
				continue
			}

			// One etmemd project per process: etmemd rejects obj add for existing
			// project names and only supports one [task] per config file.
			for _, proc := range processes {
				projectName := agent.ProjectNameForProcess(policy.Namespace, pod.Name, proc.Name)
				configContent := slideEngine.GenerateConfig(projectName, proc, params)

				desiredTasks[projectName] = desiredTaskInfo{
					Request: agent.TaskRequest{
						ProjectName:   projectName,
						ConfigContent: configContent,
					},
					PolicyRef: etmemv1.PolicyReference{
						Name:      policy.Name,
						Namespace: policy.Namespace,
					},
					PodName:     pod.Name,
					PodUID:      string(pod.UID),
					ProcessName: proc.Name,
				}
			}
		}
	}

	// 4. Diff: stop tasks no longer desired
	for _, name := range tm.RunningTasks() {
		if _, ok := desiredTasks[name]; !ok {
			if err := tm.StopTask(ctx, name); err != nil {
				logger.Error(err, "failed to stop task during reconcile diff", "project", name)
			}
		}
	}

	// 5. Start new tasks
	for _, taskInfo := range desiredTasks {
		if !tm.IsRunning(taskInfo.Request.ProjectName) {
			if err := tm.StartTask(ctx, taskInfo.Request); err != nil {
				logger.Error(err, "failed to start task", "project", taskInfo.Request.ProjectName)
			} else {
				logger.Info("Task started successfully", "project", taskInfo.Request.ProjectName, "pod", taskInfo.PodName)
			}
		}
	}

	// C3: Update metrics
	// Count unique Pods (not projects) for the metric
	uniquePods := make(map[string]bool)
	for _, taskInfo := range desiredTasks {
		uniquePods[taskInfo.PodName] = true
	}
	agent.ManagedPodsTotal.Set(float64(len(uniquePods)))

	// C3: Write EtmemNodeState
	nodeTasks := make([]etmemv1.NodeTask, 0, len(desiredTasks))
	for projectName, taskInfo := range desiredTasks {
		nodeTasks = append(nodeTasks, etmemv1.NodeTask{
			ProjectName: projectName,
			PolicyRef:   taskInfo.PolicyRef,
			PodName:     taskInfo.PodName,
			PodUID:      taskInfo.PodUID,
			Processes: []etmemv1.ManagedProcess{
				{Name: taskInfo.ProcessName},
			},
			State: "running",
		})
	}
	nodeStatus := &etmemv1.EtmemNodeStateStatus{
		NodeName: nodeName,
		Tasks:    nodeTasks,
		Metrics:  &etmemv1.NodeMetrics{TotalManagedPods: len(uniquePods)},
	}
	if err := nsWriter.WriteStatus(ctx, nodeStatus); err != nil {
		logger.Error(err, "failed to write NodeState")
	}

	return nil
}
