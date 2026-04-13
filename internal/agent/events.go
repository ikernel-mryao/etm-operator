package agent

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

const (
	EventTaskStarted           = "TaskStarted"
	EventTaskStopped           = "TaskStopped"
	EventTaskReconciled        = "TaskReconciled"
	EventCircuitBreakerTripped = "CircuitBreakerTripped"
	EventEtmemdUnreachable     = "EtmemdUnreachable"
	EventPolicySuspended       = "PolicySuspended"
)

func RecordTaskStarted(recorder record.EventRecorder, obj *corev1.Pod, projectName string) {
	recorder.Eventf(obj, corev1.EventTypeNormal, EventTaskStarted,
		"etmem task %q started for pod", projectName)
}

func RecordTaskStopped(recorder record.EventRecorder, obj *corev1.Pod, projectName string) {
	recorder.Eventf(obj, corev1.EventTypeNormal, EventTaskStopped,
		"etmem task %q stopped for pod", projectName)
}

func RecordCircuitBreakerTripped(recorder record.EventRecorder, obj *corev1.Pod, reason string) {
	recorder.Eventf(obj, corev1.EventTypeWarning, EventCircuitBreakerTripped,
		"circuit breaker tripped: %s", reason)
}

func RecordEtmemdUnreachable(recorder record.EventRecorder, obj *corev1.Pod, err error) {
	recorder.Eventf(obj, corev1.EventTypeWarning, EventEtmemdUnreachable,
		"etmemd unreachable: %v", err)
}

func RecordPolicySuspended(recorder record.EventRecorder, obj *corev1.Pod, policyName string) {
	recorder.Eventf(obj, corev1.EventTypeNormal, EventPolicySuspended,
		fmt.Sprintf("policy %q suspended, stopping task", policyName))
}
