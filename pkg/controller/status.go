// Package controller provides a Kubernetes controller for a Caffe2Job resource.
package controller

import (
	"fmt"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
)

const (
	// caffe2JobCreatedReason is added in a caffe2job when it is created.
	caffe2JobCreatedReason = "Caffe2JobCreated"
	// caffe2JobSucceededReason is added in a caffe2job when it is succeeded.
	caffe2JobSucceededReason = "Caffe2JobSucceeded"
	// caffe2JobSucceededReason is added in a caffe2job when it is running.
	caffe2JobRunningReason = "Caffe2JobRunning"
	// caffe2JobSucceededReason is added in a caffe2job when it is failed.
	caffe2JobFailedReason = "Caffe2JobFailed"
)

// updateStatus updates the status of the caffe2job.
func (tc *Controller) updateStatus(job *api.Caffe2Job, replicas int) error {
	// Expect to have `replicas - succeeded` pods alive.
	expected := replicas - int(job.Status.ReplicaStatuses.Succeeded)
	running := int(job.Status.ReplicaStatuses.Active)
	failed := int(job.Status.ReplicaStatuses.Failed)

	// All workers are running, set StartTime.
	if running == replicas {
		now := metav1.Now()
		job.Status.StartTime = &now
	}

	// Some workers are still running, leave a running condition.
	if running > 0 {
		msg := fmt.Sprintf("Caffe2Job %s is running.", job.Name)
		err := tc.updateCaffe2JobConditions(job, api.Caffe2JobRunning, caffe2JobRunningReason, msg)
		if err != nil {
			glog.Errorf("Append caffe2job condition error: %v", err)
			return err
		}
	}

	// All workers are succeeded, leave a succeeded condition.
	if expected == 0 {
		msg := fmt.Sprintf("Caffe2Job %s is successfully completed.", job.Name)
		now := metav1.Now()
		job.Status.CompletionTime = &now
		err := tc.updateCaffe2JobConditions(job, api.Caffe2JobSucceeded, caffe2JobSucceededReason, msg)
		if err != nil {
			glog.Errorf("Append caffe2job condition error: %v", err)
			return err
		}
	}

	// Some workers are failed , leave a failed condition.
	if failed > 0 {
		msg := fmt.Sprintf("Caffe2Job %s is failed.", job.Name)
		err := tc.updateCaffe2JobConditions(job, api.Caffe2JobFailed, caffe2JobFailedReason, msg)
		if err != nil {
			glog.Errorf("Append caffe2job condition error: %v", err)
			return err
		}
	}
	return nil
}

// initializeCaffe2ReplicaStatuses initializes the Caffe2ReplicaStatuses for replica.
func initializeCaffe2ReplicaStatuses(job *api.Caffe2Job) {
	job.Status.ReplicaStatuses = &api.Caffe2ReplicaStatus{}
}

// updateCaffe2JobReplicaStatuses updates the Caffe2JobReplicaStatuses according to the pod.
func updateCaffe2JobReplicaStatuses(job *api.Caffe2Job, pod *v1.Pod) {
	switch pod.Status.Phase {
	case v1.PodRunning, v1.PodPending:
		job.Status.ReplicaStatuses.Active++
	case v1.PodSucceeded:
		job.Status.ReplicaStatuses.Succeeded++
	case v1.PodFailed:
		job.Status.ReplicaStatuses.Failed++
	}
}

// increaseCaffe2JobReplicaStatusesActive increases active in Caffe2JobReplicaStatuses.
func increaseCaffe2JobReplicaStatusesActive(job *api.Caffe2Job) {
	job.Status.ReplicaStatuses.Active++
}
