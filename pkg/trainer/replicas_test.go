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
	"fmt"
	"reflect"
	"testing"
	"time"

	api "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
	caffe2JobFake "github.com/kubeflow/caffe2-operator/pkg/client/clientset/versioned/fake"
	"github.com/kubeflow/caffe2-operator/pkg/util"

	"github.com/golang/protobuf/proto"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
)

var (
	groupVersionKind = schema.GroupVersionKind{
		Group:   api.GroupName,
		Version: api.GroupVersion,
		Kind:    api.Caffe2JobResourceKind,
	}
)

func TestCaffe2ReplicaSet(t *testing.T) {
	clientSet := fake.NewSimpleClientset()

	jobSpec := &api.Caffe2Job{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "some-job",
			UID:  "some-uid",
		},
		Spec: api.Caffe2JobSpec{
			RuntimeId: "some-runtime",
			ReplicaSpecs: []*api.Caffe2ReplicaSpec{
				{
					Replicas:   proto.Int32(2),
					Caffe2Port: proto.Int32(10),
					Template: &v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name: "caffe2",
								},
							},
						},
					},
					Caffe2ReplicaType: api.PS,
				},
			},
		},
	}

	recorder := record.NewFakeRecorder(100)
	job, err := initJob(clientSet, &caffe2JobFake.Clientset{}, recorder, jobSpec)

	if err != nil {
		t.Fatalf("initJob failed: %v", err)
	}

	replica, err := NewCaffe2ReplicaSet(clientSet, recorder, *jobSpec.Spec.ReplicaSpecs[0], job)

	if err != nil {
		t.Fatalf("NewCaffe2ReplicaSet failed: %v", err)
	}

	if err := replica.Create(&api.ControllerConfig{}); err != nil {
		t.Fatalf("replica.Create() error; %v", err)
	}

	trueVal := true
	expectedOwnerReference := meta_v1.OwnerReference{
		APIVersion:         groupVersionKind.GroupVersion().String(),
		Kind:               groupVersionKind.Kind,
		Name:               "some-job",
		UID:                "some-uid",
		Controller:         &trueVal,
		BlockOwnerDeletion: &trueVal,
	}

	for index := 0; index < 2; index++ {
		// Expected labels
		expectedLabels := map[string]string{
			"kubeflow.org": "",
			"task_index":   fmt.Sprintf("%v", index),
			"job_type":     "PS",
			"runtime_id":   "some-runtime",
			"tf_job_name":  "some-job",
		}

		// Check that a service was created.
		sList, err := clientSet.CoreV1().Services(replica.Job.job.ObjectMeta.Namespace).List(meta_v1.ListOptions{})
		if err != nil {
			t.Fatalf("List services error; %v", err)
		}

		if len(sList.Items) != 2 {
			t.Fatalf("Expected 2 services got %v", len(sList.Items))
		}

		s := sList.Items[index]

		if !reflect.DeepEqual(expectedLabels, s.ObjectMeta.Labels) {
			t.Fatalf("Service Labels; Got %v Want: %v", s.ObjectMeta.Labels, expectedLabels)
		}

		name := fmt.Sprintf("some-job-ps-some-runtime-%v", index)
		if s.ObjectMeta.Name != name {
			t.Fatalf("Job.ObjectMeta.Name = %v; want %v", s.ObjectMeta.Name, name)
		}

		if len(s.ObjectMeta.OwnerReferences) != 1 {
			t.Fatalf("Expected 1 owner reference got %v", len(s.ObjectMeta.OwnerReferences))
		}

		if !reflect.DeepEqual(s.ObjectMeta.OwnerReferences[0], expectedOwnerReference) {
			t.Fatalf("Service.Metadata.OwnerReferences; Got %v; want %v", util.Pformat(s.ObjectMeta.OwnerReferences[0]), util.Pformat(expectedOwnerReference))
		}

		// Check that a job was created.
		l, err := clientSet.BatchV1().Jobs(replica.Job.job.ObjectMeta.Namespace).List(meta_v1.ListOptions{})
		if err != nil {
			t.Fatalf("List jobs error; %v", err)
		}

		if len(l.Items) != 2 {
			t.Fatalf("Expected 1 job got %v", len(l.Items))
		}

		j := l.Items[index]

		if !reflect.DeepEqual(expectedLabels, j.ObjectMeta.Labels) {
			t.Fatalf("Job Labels; Got %v Want: %v", expectedLabels, j.ObjectMeta.Labels)
		}

		if j.ObjectMeta.Name != name {
			t.Fatalf("Job.ObjectMeta.Name = %v; want %v", j.ObjectMeta.Name, name)
		}

		if len(j.Spec.Template.Spec.Containers) != 1 {
			t.Fatalf("Expected 1 container got %v", len(j.Spec.Template.Spec.Containers))
		}

		if len(j.ObjectMeta.OwnerReferences) != 1 {
			t.Fatalf("Expected 1 owner reference got %v", len(j.ObjectMeta.OwnerReferences))
		}

		if !reflect.DeepEqual(j.ObjectMeta.OwnerReferences[0], expectedOwnerReference) {
			t.Fatalf("Job.Metadata.OwnerReferences; Got %v; want %v", util.Pformat(j.ObjectMeta.OwnerReferences[0]), util.Pformat(expectedOwnerReference))
		}

		c := j.Spec.Template.Spec.Containers[0]
		if len(c.Env) != 1 {
			t.Fatalf("Expected 1 environment variable got %v", len(c.Env))
		}

		actualCaffe2Config := &Caffe2Config{}
		if err := json.Unmarshal([]byte(c.Env[0].Value), actualCaffe2Config); err != nil {
			t.Fatalf("Could not unmarshal Caffe2Config %v", err)
		}

		expectedCaffe2Config := &Caffe2Config{
			Cluster: ClusterSpec{},
			Task: TaskSpec{
				Type:  "ps",
				Index: index,
			},
			Environment: "cloud",
		}

		if !reflect.DeepEqual(expectedCaffe2Config, actualCaffe2Config) {
			t.Fatalf("Got %v, Want %v", actualCaffe2Config, expectedCaffe2Config)
		}
	}
	// Delete the job.
	// N.B it doesn't look like the Fake clientset is sophisticated enough to delete jobs in response to a
	// DeleteCollection request (deleting individual jobs does appear to work with the Fake). So if we were to list
	// the jobs after calling Delete we'd still see the job. So we will rely on E2E tests to verify Delete works
	// correctly.
	if err := replica.Delete(); err != nil {
		t.Fatalf("replica.Delete() error; %v", err)
	}
}

func TestCaffe2ReplicaSetStatusFromPodList(t *testing.T) {
	type TestCase struct {
		PodList  v1.PodList
		Name     string
		Expected api.ReplicaState
	}

	cases := []TestCase{
		{
			PodList: v1.PodList{
				Items: []v1.Pod{
					{
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name: "master",
									State: v1.ContainerState{
										Running: &v1.ContainerStateRunning{},
									},
								},
							},
						},
					},
				},
			},
			Name:     "master",
			Expected: api.ReplicaStateRunning,
		},
		{
			PodList: v1.PodList{
				Items: []v1.Pod{
					{
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name: "master",
									State: v1.ContainerState{
										Terminated: &v1.ContainerStateTerminated{
											ExitCode: 0,
										},
									},
								},
							},
						},
					},
				},
			},
			Name:     "master",
			Expected: api.ReplicaStateSucceeded,
		},
		{
			// Multiple containers; make sure we match by name.
			PodList: v1.PodList{
				Items: []v1.Pod{
					{
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name: "other",
									State: v1.ContainerState{
										Running: &v1.ContainerStateRunning{},
									},
								},
								{
									Name: "master",
									State: v1.ContainerState{
										Terminated: &v1.ContainerStateTerminated{
											ExitCode: 0,
										},
									},
								},
							},
						},
					},
				},
			},
			Name:     "master",
			Expected: api.ReplicaStateSucceeded,
		},
		{
			// Container failed with permanent error and then got restarted.
			PodList: v1.PodList{
				Items: []v1.Pod{
					{
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name: "master",
									State: v1.ContainerState{
										Running: &v1.ContainerStateRunning{},
									},
									LastTerminationState: v1.ContainerState{
										Terminated: &v1.ContainerStateTerminated{
											ExitCode: 100,
											Message:  "some reason",
										},
									},
								},
							},
						},
					},
				},
			},
			Name:     "master",
			Expected: api.ReplicaStateFailed,
		},
		{
			// Multiple Pods; check we get the most recent.
			PodList: v1.PodList{
				Items: []v1.Pod{
					{
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name: "master",
									State: v1.ContainerState{
										Running: &v1.ContainerStateRunning{},
									},
								},
							},
							StartTime: &meta_v1.Time{
								Time: time.Date(2017, 0, 0, 0, 0, 0, 0, time.UTC),
							},
						},
					},
					{
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name: "master",
									State: v1.ContainerState{
										Terminated: &v1.ContainerStateTerminated{
											ExitCode: 100,
											Message:  "some reason",
										},
									},
								},
							},
							StartTime: &meta_v1.Time{
								Time: time.Date(2018, 0, 0, 0, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			},
			Name:     "master",
			Expected: api.ReplicaStateFailed,
		},
	}

	for _, c := range cases {
		status := replicaStatusFromPodList(c.PodList, api.ContainerName(c.Name))
		if status != c.Expected {
			t.Errorf("replicaStatusFromPodList(%+v, %v)=%v ; want %v", c.PodList, c.Name, status, c.Expected)
		}
	}
}
