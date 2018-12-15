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

package v1alpha1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CRDKind       = "caffe2job"
	CRDKindPlural = "caffe2jobs"
	CRDGroup      = "kubeflow.org"
	CRDVersion    = "v1alpha1"
	// Value of the APP label that gets applied to a lot of entities.
	AppLabel = "caffe2-job"
	// Defaults for the Spec
	Caffe2Port         = 2222
	Replicas           = 1
	CAFFE2             = "caffe2"
	DefaultCaffe2Image = "kubeflow/caffe2:py2-cuda9.0-cudnn7-ubuntu16.04"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=caffe2job

// Caffe2Job describes caffe2job info
type Caffe2Job struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object's metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the Caffe2Job.
	Spec Caffe2JobSpec `json:"spec"`

	// Most recently observed status of the Caffe2Job
	Status Caffe2JobStatus `json:"status"`
}

type Caffe2JobSpec struct {
	// RuntimeID
	RuntimeID string

	// Backend specifies the nodes’ communications tool
	Backend *Caffe2BackendSpec `json:"backendSpecs"`

	// ReplicaSpecs specifies the Caffe2 replicas to run.
	ReplicaSpecs *Caffe2ReplicaSpec `json:"replicaSpecs"`

	// TerminationPolicy specifies the condition that the caffe2job should be considered finished.
	TerminationPolicy *TerminationPolicySpec `json:"terminationPolicy,omitempty"`
}

// Caffe2BackendSpec
type Caffe2BackendSpec struct {
	Type      BackendType `json:"backendType"`
	NFSPath   string      `json:"nfsPath"`
	RedisHost string      `json:"redisHost"`
	RedisPort uint        `json:"redisPort"`
}

// BackendType backend type
type BackendType string

var (
	NoneBackendType  BackendType = "none"
	RedisBackendType BackendType = "redis"
	NFSBackendType   BackendType = "nfs"
)

// TerminationPolicySpec
type TerminationPolicySpec struct {
	// Chief policy waits for a particular process (which is the chief) to exit.
	Chief *ChiefSpec `json:"chief,omitempty"`
}

type ChiefSpec struct {
	ReplicaName  string `json:"replicaName"`
	ReplicaIndex int    `json:"replicaIndex"`
}

type Caffe2ReplicaSpec struct {
	// Replicas is the number of desired replicas.
	// This is a pointer to distinguish between explicit zero and unspecified.
	// Defaults to 1.
	// More info: http://kubernetes.io/docs/user-guide/replication-controller#what-is-a-replication-controller
	// +optional
	Replicas *int32 `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`

	//  Template is the object that describes the pod that
	// will be created for this Caffe2Replica.
	Template *v1.PodTemplateSpec `json:"template,omitempty" protobuf:"bytes,3,opt,name=template"`
}

type Caffe2JobStatus struct {
	// Conditions is an array of current observed Caffe2Job conditions.
	Conditions []Caffe2JobCondition `json:"conditions"`

	// ReplicaStatuses specifies the status of each Caffe2 replica.
	ReplicaStatuses *Caffe2ReplicaStatus `json:"replicaStatuses"`

	// Represents time when the Caffe2Job was acknowledged by the Caffe2Job controller.
	// It is not guaranteed to be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Represents time when the Caffe2Job was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Represents last time when the Caffe2Job was reconciled. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// Caffe2ReplicaStatus represents the current observed state of the Caffe2Replica.
type Caffe2ReplicaStatus struct {
	// The number of actively running pods.
	Active int32 `json:"active,omitempty"`

	// The number of pods which reached phase Succeeded.
	Succeeded int32 `json:"succeeded,omitempty"`

	// The number of pods which reached phase Failed.
	Failed int32 `json:"failed,omitempty"`
}

// Caffe2JobCondition describes the state of the Caffe2Job at a certain point.
type Caffe2JobCondition struct {
	// Type of Caffe2Job condition.
	Type Caffe2JobConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// Caffe2JobConditionType defines all kinds of types of Caffe2JobStatus.
type Caffe2JobConditionType string

const (
	// Caffe2JobCreated means the caffe2job has been accepted by the system,
	// but one or more of the pods/services has not been started.
	// This includes time before pods being scheduled and launched.
	Caffe2JobCreated Caffe2JobConditionType = "Created"

	// Caffe2JobRunning means all sub-resources (e.g. services/pods) of this Caffe2Job
	// have been successfully scheduled and launched.
	// The training is running without error.
	Caffe2JobRunning Caffe2JobConditionType = "Running"

	// Caffe2JobSucceeded means all sub-resources (e.g. services/pods) of this Caffe2Job
	// reached phase have terminated in success.
	// The training is complete without error.
	Caffe2JobSucceeded Caffe2JobConditionType = "Succeeded"

	// Caffe2JobFailed means one or more sub-resources (e.g. services/pods) of this Caffe2Job
	// reached phase failed with no restarting.
	// The training has failed its execution.
	Caffe2JobFailed Caffe2JobConditionType = "Failed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=caffe2jobs

// Caffe2JobList is a list of Caffe2Jobs clusters.
type Caffe2JobList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of Caffe2Jobs
	Items []Caffe2Job `json:"items"`
}
