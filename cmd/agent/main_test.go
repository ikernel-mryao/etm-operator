package main

import (
	"testing"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

func TestBuildNodeStatusSetsHealthFlagsAndUniquePodMetric(t *testing.T) {
	desired := map[string]desiredTaskInfo{
		"proj-a": {
			PolicyRef:   etmemv1.PolicyReference{Name: "etmem-auto", Namespace: "default"},
			PodName:     "pod-a",
			PodUID:      "uid-a",
			ProcessName: "dbmonitor",
			PID:         1001,
		},
		"proj-b": {
			PolicyRef:   etmemv1.PolicyReference{Name: "etmem-auto", Namespace: "default"},
			PodName:     "pod-a",
			PodUID:      "uid-a",
			ProcessName: "zengine",
			PID:         1002,
		},
		"proj-c": {
			PolicyRef:   etmemv1.PolicyReference{Name: "etmem-auto", Namespace: "default"},
			PodName:     "pod-b",
			PodUID:      "uid-b",
			ProcessName: "worker",
			PID:         1003,
		},
	}

	status := buildNodeStatus("cp0", desired, true)

	if !status.SocketReachable {
		t.Fatalf("expected SocketReachable=true")
	}
	if !status.EtmemdReady {
		t.Fatalf("expected EtmemdReady=true")
	}
	if status.NodeName != "cp0" {
		t.Fatalf("expected node name cp0, got %q", status.NodeName)
	}
	if status.Metrics == nil || status.Metrics.TotalManagedPods != 2 {
		t.Fatalf("expected TotalManagedPods=2, got %+v", status.Metrics)
	}
	if len(status.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(status.Tasks))
	}
}
