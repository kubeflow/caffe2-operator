// Copyright 2018 The Kubeflow Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package trainer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/helper"
	api "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
	"github.com/kubeflow/caffe2-operator/pkg/util"

	log "github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	batch "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sErrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
)

const (
	SuccessfulCreateReason = "SuccessfulCreate"
	FailedCreateReason     = "FailedCreate"
)

// Caffe2ReplicaSet is a set of Caffe2 processes all acting as the same role (e.g. worker
type Caffe2ReplicaSet struct {
	ClientSet kubernetes.Interface
	recorder  record.EventRecorder
	// Job is a pointer to the TrainingJob to which this replica belongs.
	Job  *TrainingJob
	Spec api.Caffe2ReplicaSpec
}

// Caffe2Replicas is an interface for managing a set of replicas.
type Caffe2ReplicaSetInterface interface {
	Create() error
	Delete() error
	GetStatus() (api.Caffe2ReplicaStatus, error)
}

// Caffe2Config is a struct representing the TensorFlow config. This struct is turned into an environment
// which is used by TensorFlow processes to configure themselves.
type Caffe2Config struct {
	// Cluster represents a Caffe2 ClusterSpec.
	// See: https://www.tensorflow.org/api_docs/python/tf/train/ClusterSpechttps://www.tensorflow.org/api_docs/python/tf/train/ClusterSpec
	Cluster     ClusterSpec `json:"cluster"`
	Task        TaskSpec    `json:"task"`
	Environment string      `json:"environment"`
}

func NewCaffe2ReplicaSet(clientSet kubernetes.Interface, recorder record.EventRecorder, caffe2ReplicaSpec api.Caffe2ReplicaSpec, job *TrainingJob) (*Caffe2ReplicaSet, error) {
	if caffe2ReplicaSpec.Caffe2ReplicaType == api.MASTER && *caffe2ReplicaSpec.Replicas != 1 {
		return nil, errors.New("The MASTER must have Replicas = 1")
	}

	if caffe2ReplicaSpec.Caffe2Port == nil {
		return nil, errors.New("caffe2ReplicaSpec.Caffe2Port can't be nil.")
	}

	if caffe2ReplicaSpec.Template == nil {
		return nil, fmt.Errorf("caffe2Replicatfv1alpha1.Template can't be nil for replica type %v.", caffe2ReplicaSpec.Caffe2ReplicaType)
	}

	// Make sure the replica type is valid.
	validReplicaTypes := []api.Caffe2ReplicaType{api.MASTER, api.WORKER}

	isValidReplicaType := false
	for _, t := range validReplicaTypes {
		if t == caffe2ReplicaSpec.Caffe2ReplicaType {
			isValidReplicaType = true
			break
		}
	}

	if !isValidReplicaType {
		return nil, fmt.Errorf("caffe2ReplicaSpec.Caffe2ReplicaType is %v but must be one of %v", caffe2ReplicaSpec.Caffe2ReplicaType, validReplicaTypes)
	}

	return &Caffe2ReplicaSet{
		ClientSet: clientSet,
		recorder:  recorder,
		Job:       job,
		Spec:      caffe2ReplicaSpec,
	}, nil
}

// Labels returns the labels for this replica set.
func (s *Caffe2ReplicaSet) Labels() KubernetesLabels {
	return KubernetesLabels(map[string]string{
		"kubeflow.org": "",
		"job_type":     string(s.Spec.Caffe2ReplicaType),
		// runtime_id is set by Job.setup, which is called after the Caffe2ReplicaSet is created.
		// this is why labels aren't a member variable.
		"runtime_id":      s.Job.job.Spec.RuntimeId,
		"caffe2_job_name": s.Job.job.ObjectMeta.Name})
}

func (s *Caffe2ReplicaSet) Create(config *api.ControllerConfig) error {
	for index := int32(0); index < *s.Spec.Replicas; index++ {
		taskLabels := s.Labels()
		taskLabels["task_index"] = fmt.Sprintf("%v", index)

		// Create the service.
		service := &v1.Service{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:   s.jobName(index),
				Labels: taskLabels,
				OwnerReferences: []meta_v1.OwnerReference{
					helper.AsOwner(s.Job.job),
				},
			},
			Spec: v1.ServiceSpec{
				Selector: taskLabels,
				Ports: []v1.ServicePort{
					{
						Name: "caffe2-port",
						Port: *s.Spec.Caffe2Port,
					},
				},
			},
		}

		log.Infof("Creating Service: %v", service.ObjectMeta.Name)
		createdService, err := s.ClientSet.CoreV1().Services(s.Job.job.ObjectMeta.Namespace).Create(service)

		// If the job already exists do nothing.
		if err != nil {
			if k8s_errors.IsAlreadyExists(err) {
				log.Infof("Service %v already exists.", s.jobName(index))
			} else {
				s.recorder.Eventf(s.Job.job, v1.EventTypeWarning, FailedCreateReason, "Error creating: %v", err)
				return k8sErrors.NewAggregate([]error{fmt.Errorf("Creating service %v returned error.", createdService.ObjectMeta.Name), err})
			}
		} else {
			s.recorder.Eventf(s.Job.job, v1.EventTypeNormal, SuccessfulCreateReason, "Created service: %v", createdService.Name)
		}

		// Configure the TFCONFIG environment variable.
		caffe2Config := Caffe2Config{
			Cluster: s.Job.ClusterSpec(),
			Task: TaskSpec{
				Type:  strings.ToLower(string(s.Spec.Caffe2ReplicaType)),
				Index: int(index),
			},
			// We need to set environment to cloud  otherwise it will default to local which isn't what we want.
			Environment: "cloud",
		}

		caffe2ConfigJson, err := json.Marshal(caffe2Config)
		if err != nil {
			log.Errorf("Job: %v serializing caffe2Config: %v return error; %v", s.Job.job.ObjectMeta.Name, util.Pformat(caffe2Config), err)
			return err
		}

		// Make a copy of the template because we will modify it below. .
		newPodSpecTemplate := s.Spec.Template.DeepCopy()

		newJ := &batch.Job{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:   s.jobName(index),
				Labels: taskLabels,
				OwnerReferences: []meta_v1.OwnerReference{
					helper.AsOwner(s.Job.job),
				},
			},
			Spec: batch.JobSpec{
				Completions: proto.Int32(1),
				Parallelism: proto.Int32(1),
				Template:    *newPodSpecTemplate,
			},
		}

		if newJ.Spec.Template.ObjectMeta.Labels == nil {
			newJ.Spec.Template.ObjectMeta.Labels = make(map[string]string)
		}

		// Pods need to be tagged with the labels.
		for k, v := range taskLabels {
			newJ.Spec.Template.ObjectMeta.Labels[k] = v
		}

		// Add TF_CONFIG environment variable.
		for i, _ := range newJ.Spec.Template.Spec.Containers {
			// We can't get c in the loop variable because that would be by value so our modifications
			// wouldn't have any effect.
			c := &newJ.Spec.Template.Spec.Containers[i]
			if api.ContainerName(c.Name) != api.CAFFE2 {
				continue
			}
			if len(c.Env) == 0 {
				c.Env = make([]v1.EnvVar, 0)
			}
			c.Env = append(c.Env, v1.EnvVar{
				Name:  "CAFFE2_CONFIG",
				Value: string(caffe2ConfigJson),
			})
		}

		log.Infof("Creating Job: %#v", newJ.ObjectMeta)
		createdJob, err := s.ClientSet.BatchV1().Jobs(s.Job.job.ObjectMeta.Namespace).Create(newJ)

		// If the job already exists do nothing.
		if err != nil {
			if k8s_errors.IsAlreadyExists(err) {
				log.Infof("%v already exists.", s.jobName(index))

			} else {
				s.recorder.Eventf(s.Job.job, v1.EventTypeWarning, FailedCreateReason, "Error creating: %v", err)
				return k8sErrors.NewAggregate([]error{fmt.Errorf("Creating Job %v returned error.", createdJob.ObjectMeta.Name), err})
			}
		} else {
			s.recorder.Eventf(s.Job.job, v1.EventTypeNormal, SuccessfulCreateReason, "Created job: %#v", createdJob)
		}
	}
	return nil
}

// Delete deletes the replicas
func (s *Caffe2ReplicaSet) Delete() error {
	selector, err := s.Labels().ToSelector()
	if err != nil {
		return err
	}

	failures := false

	options := meta_v1.ListOptions{
		LabelSelector: selector,
	}

	log.V(1).Infof("Deleting Jobs namespace=%v selector=%v", s.Job.job.ObjectMeta.Namespace, selector)
	err = s.ClientSet.BatchV1().Jobs(s.Job.job.ObjectMeta.Namespace).DeleteCollection(&meta_v1.DeleteOptions{}, options)

	if err != nil {
		log.Errorf("There was a problem deleting the jobs; %v", err)
		failures = true
	}

	// We need to delete the completed pods.
	log.V(1).Infof("Deleting Pods namespace=%v selector=%v", s.Job.job.ObjectMeta.Namespace, selector)
	err = s.ClientSet.CoreV1().Pods(s.Job.job.ObjectMeta.Namespace).DeleteCollection(&meta_v1.DeleteOptions{}, options)

	if err != nil {
		log.Errorf("There was a problem deleting the pods; %v", err)
		failures = true
	}

	// Services doesn't support DeleteCollection so we delete them individually.
	// TODO: We should check if this has changed with K8s 1.8 or other releases.
	for index := int32(0); index < *s.Spec.Replicas; index++ {
		log.V(1).Infof("Deleting Service %v:%v", s.Job.job.ObjectMeta.Namespace, s.jobName((index)))
		err = s.ClientSet.CoreV1().Services(s.Job.job.ObjectMeta.Namespace).Delete(s.jobName(index), &meta_v1.DeleteOptions{})

		if err != nil {
			log.Errorf("Error deleting service %v; %v", s.jobName(index), err)
			failures = true
		}
	}

	if failures {
		return errors.New("Some of the replicas resources could not be deleted")
	}
	return nil
}

// replicaStatusFromPodList returns a status from a list of pods for a job.
func replicaStatusFromPodList(l v1.PodList, name api.ContainerName) api.ReplicaState {
	log.V(1).Infof("Get replicaStatus from PodList: %v", util.Pformat(l))
	var latest *v1.Pod
	for _, i := range l.Items {
		if latest == nil {
			latest = &i
			continue
		}
		if latest.Status.StartTime == nil || i.Status.StartTime == nil {
			continue
		}
		if latest.Status.StartTime.Before(i.Status.StartTime) {
			latest = &i
		}
	}

	if latest == nil {
		return api.ReplicaStateRunning
	}

	var caffe2State v1.ContainerState

	for _, i := range latest.Status.ContainerStatuses {
		if i.Name != string(name) {
			continue
		}

		// We need to decide whether to use the current state or the previous termination state.
		caffe2State = i.State

		// If the container previously terminated we will look at the termination to decide whether it is a retryable
		// or permanenent error.
		if i.LastTerminationState.Terminated != nil {
			caffe2State = i.LastTerminationState
		}
	}

	if caffe2State.Running != nil || caffe2State.Waiting != nil {
		return api.ReplicaStateRunning
	}

	if caffe2State.Terminated != nil {
		if caffe2State.Terminated.ExitCode == 0 {
			return api.ReplicaStateSucceeded
		}

		if isRetryableTerminationState(caffe2State.Terminated) {
			// Since its a retryable error just return RUNNING.
			// We can just let Kubernetes restart the container to retry.
			return api.ReplicaStateRunning
		}

		return api.ReplicaStateFailed
	}

	return api.ReplicaStateUnknown
}

func (s *Caffe2ReplicaSet) GetSingleReplicaStatus(index int32) api.ReplicaState {
	j, err := s.ClientSet.BatchV1().Jobs(s.Job.job.ObjectMeta.Namespace).Get(s.jobName(index), meta_v1.GetOptions{})

	if err != nil {
		return api.ReplicaStateUnknown
	}

	if j.Status.Succeeded >= 1 {
		return api.ReplicaStateSucceeded
	}

	labels := s.Labels()
	labels["task_index"] = fmt.Sprintf("%v", index)
	selector, err := labels.ToSelector()
	if err != nil {
		log.Errorf("labels.ToSelector() error; %v", err)
		return api.ReplicaStateFailed
	}

	// TODO: Handle errors. We need to get the pod and looking at recent container exits.
	l, err := s.ClientSet.CoreV1().Pods(s.Job.job.ObjectMeta.Namespace).List(meta_v1.ListOptions{
		// TODO: Why isn't the label selector working?
		LabelSelector: selector,
	})

	if err != nil {
		// TODO: Are there errors that should be treated as retryable errors?
		return api.ReplicaStateFailed
	}

	status := replicaStatusFromPodList(*l, api.CAFFE2)
	return status
}

// Status returns the status of the replica set.
func (s *Caffe2ReplicaSet) GetStatus() (api.Caffe2ReplicaStatus, error) {
	status := api.Caffe2ReplicaStatus{
		Caffe2ReplicaType: s.Spec.Caffe2ReplicaType,
		State:             api.ReplicaStateUnknown,
		ReplicasStates:    make(map[api.ReplicaState]int),
	}

	increment := func(state api.ReplicaState) {
		v, ok := status.ReplicasStates[state]
		if ok {
			status.ReplicasStates[state] = v + 1
		} else {
			status.ReplicasStates[state] = 1
		}
	}

	for index := int32(0); index < *s.Spec.Replicas; index++ {
		increment(s.GetSingleReplicaStatus(index))
	}

	// Determine the overall status for the replica set based on the status of the individual replicas.
	// If any of the replicas failed mark the set as failed.
	if _, ok := status.ReplicasStates[api.ReplicaStateFailed]; ok {
		status.State = api.ReplicaStateFailed
		return status, nil
	}

	// If any replicas are RUNNING mark it as RUNNING.
	if _, ok := status.ReplicasStates[api.ReplicaStateRunning]; ok {
		status.State = api.ReplicaStateRunning
		return status, nil
	}

	// If all of the replicas succeeded consider it success.
	if v, ok := status.ReplicasStates[api.ReplicaStateSucceeded]; ok && int32(v) == *s.Spec.Replicas {
		status.State = api.ReplicaStateSucceeded
		return status, nil
	}

	return status, nil
}

func (s *Caffe2ReplicaSet) jobName(index int32) string {
	// Truncate caffe2job name to 40 characters
	// The whole job name should be compliant with the DNS_LABEL spec, up to a max length of 63 characters
	// Thus jobname(40 chars)-replicaType(6 chars)-runtimeId(4 chars)-index(4 chars), also leaving some spaces
	// See https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/identifiers.md
	return fmt.Sprintf("%v-%v-%v-%v", fmt.Sprintf("%.40s", s.Job.job.ObjectMeta.Name), strings.ToLower(string(s.Spec.Caffe2ReplicaType)), s.Job.job.Spec.RuntimeId, index)
}
