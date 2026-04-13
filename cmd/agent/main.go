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

	executor := &transport.RealExecutor{}
	tr := transport.NewExecTransport(socketName, executor)
	tm := agent.NewTaskManager(tr, config.EtmemConfigDir)
	pidResolver := agent.NewPIDResolver(config.HostProcPath, config.HostCgroupPath)
	slideEngine := &engine.SlideEngine{}

	// TODO: Phase 3 — initialize CircuitBreaker (Task 3.1)
	// TODO: Phase 3 — initialize Prometheus metrics (Task 3.4)

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

	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Agent stopped")
			return
		case <-ticker.C:
			if err := agentReconcile(ctx, k8sClient, tm, pidResolver, slideEngine, nodeName, logger); err != nil {
				logger.Error(err, "reconcile failed")
			}
		}
	}
}

func agentReconcile(
	ctx context.Context,
	k8sClient client.Client,
	tm *agent.TaskManager,
	pidResolver *agent.PIDResolver,
	slideEngine *engine.SlideEngine,
	nodeName string,
	logger logr.Logger,
) error {
	// TODO: Phase 3 — check node-level circuit breaker (Task 3.1)

	// 1. List all EtmemPolicies
	var policyList etmemv1.EtmemPolicyList
	if err := k8sClient.List(ctx, &policyList); err != nil {
		return fmt.Errorf("list policies: %w", err)
	}

	// 2. List Pods on this node
	var podList corev1.PodList
	if err := k8sClient.List(ctx, &podList, client.MatchingFields{"spec.nodeName": nodeName}); err != nil {
		return fmt.Errorf("list pods: %w", err)
	}

	// 3. Build desired task set
	desiredTasks := make(map[string]agent.TaskRequest)

	for i := range policyList.Items {
		policy := &policyList.Items[i]
		if policy.Spec.Suspend {
			continue
		}
		params, err := engine.GetProfile(policy.Spec.Engine.Profile)
		if err != nil {
			logger.Error(err, "invalid profile", "policy", policy.Name, "profile", policy.Spec.Engine.Profile)
			continue
		}
		params = engine.ApplyOverrides(params, policy.Spec.Engine.Overrides)

		for j := range podList.Items {
			pod := &podList.Items[j]
			if !agent.MatchPodToPolicy(pod, policy, nodeName, nil) {
				continue
			}

			// TODO: Phase 3 — check pod-level circuit breaker (Task 3.1)

			pids, err := pidResolver.ResolvePIDs(string(pod.UID), policy.Spec.ProcessFilter.Names)
			if err != nil || len(pids) == 0 {
				continue
			}

			processes := make([]engine.ProcessTarget, 0, len(pids))
			for _, pid := range pids {
				processes = append(processes, engine.ProcessTarget{Name: pid.Name})
			}
			projectName := agent.ProjectName(policy.Namespace, pod.Name)
			configContent := slideEngine.GenerateConfig(projectName, processes, params)
			desiredTasks[projectName] = agent.TaskRequest{
				ProjectName:   projectName,
				ConfigContent: configContent,
			}
		}
	}

	// 4. Diff: stop tasks no longer desired
	for _, name := range tm.RunningTasks() {
		if _, ok := desiredTasks[name]; !ok {
			_ = tm.StopTask(ctx, name)
		}
	}

	// 5. Start new tasks
	for _, req := range desiredTasks {
		if !tm.IsRunning(req.ProjectName) {
			if err := tm.StartTask(ctx, req); err != nil {
				logger.Error(err, "failed to start task", "project", req.ProjectName)
			}
		}
	}

	// TODO: Phase 3 — update metrics (Task 3.4)
	// TODO: Phase 3 — write EtmemNodeState (Task 3.2)

	return nil
}
