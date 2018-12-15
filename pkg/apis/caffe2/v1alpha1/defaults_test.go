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
	"reflect"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/kubeflow/caffe2-operator/pkg/util"
	"k8s.io/api/core/v1"
)

func TestSetDefaults_Caffe2Job(t *testing.T) {
	type testCase struct {
		in       *Caffe2Job
		expected *Caffe2Job
	}

	testCases := []testCase{
		{
			in: &Caffe2Job{
				Spec: Caffe2JobSpec{
					ReplicaSpecs: &Caffe2ReplicaSpec{
						Template: &v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name: "tensorflow",
									},
								},
							},
						},
					},
				},
			},
			expected: &Caffe2Job{
				Spec: Caffe2JobSpec{
					Backend: &Caffe2BackendSpec{
						Type: NoneBackendType,
					},
					ReplicaSpecs: &Caffe2ReplicaSpec{
						Replicas: proto.Int32(1),
						Template: &v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name: "tensorflow",
									},
								},
								RestartPolicy: "Never",
							},
						},
					},
					TerminationPolicy: &TerminationPolicySpec{
						Chief: &ChiefSpec{
							ReplicaName:  "WORKER",
							ReplicaIndex: 0,
						},
					},
				},
			},
		},
		{
			in: &Caffe2Job{
				Spec: Caffe2JobSpec{
					ReplicaSpecs: &Caffe2ReplicaSpec{},
				},
			},
			expected: &Caffe2Job{
				Spec: Caffe2JobSpec{
					Backend: &Caffe2BackendSpec{
						Type: NoneBackendType,
					},
					ReplicaSpecs: &Caffe2ReplicaSpec{
						Replicas: proto.Int32(1),
					},
					TerminationPolicy: &TerminationPolicySpec{
						Chief: &ChiefSpec{
							ReplicaName:  "WORKER",
							ReplicaIndex: 0,
						},
					},
				},
			},
		},
	}

	for _, c := range testCases {
		SetDefaults_Caffe2Job(c.in)
		c.in.Spec.RuntimeID = c.expected.Spec.RuntimeID
		if !reflect.DeepEqual(c.in, c.expected) {
			t.Errorf("Want\n%v; Got\n %v", util.Pformat(c.expected), util.Pformat(c.in))
		}
	}
}
