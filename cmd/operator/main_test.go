package main

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

type fakeProbeManager struct {
	healthzNames []string
	readyzNames  []string
}

func (f *fakeProbeManager) AddHealthzCheck(name string, _ healthz.Checker) error {
	f.healthzNames = append(f.healthzNames, name)
	return nil
}

func (f *fakeProbeManager) AddReadyzCheck(name string, _ healthz.Checker) error {
	f.readyzNames = append(f.readyzNames, name)
	return nil
}

func TestRegisterProbesRegistersHealthzAndReadyz(t *testing.T) {
	mgr := &fakeProbeManager{}

	if err := registerProbes(mgr); err != nil {
		t.Fatalf("registerProbes() error = %v", err)
	}

	if len(mgr.healthzNames) != 1 || mgr.healthzNames[0] != "healthz" {
		t.Fatalf("expected healthz check to be registered once, got %v", mgr.healthzNames)
	}

	if len(mgr.readyzNames) != 1 || mgr.readyzNames[0] != "readyz" {
		t.Fatalf("expected readyz check to be registered once, got %v", mgr.readyzNames)
	}
}
