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

package helper

import (
	"reflect"
	"testing"

	"github.com/gogo/protobuf/proto"
	api "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
	"github.com/kubeflow/caffe2-operator/pkg/util"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestAddAccelertor(t *testing.T) {
	type testCase struct {
		in       *api.Caffe2JobSpec
		expected *api.Caffe2JobSpec
		config   map[string]api.AcceleratorConfig
	}

	testCases := []testCase{
		// Case 1 checks that we look at requests.
		{
			in: &api.Caffe2JobSpec{
				ReplicaSpecs: []*api.TFReplicaSpec{
					{
						Replicas: proto.Int32(2),
						TFPort:   proto.Int32(10),
						Template: &v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name: "tensorflow",
										Resources: v1.ResourceRequirements{
											Requests: map[v1.ResourceName]resource.Quantity{
												"nvidia-gpu": resource.MustParse("1"),
											},
										},
									},
								},
							},
						},
						TFReplicaType: api.PS,
					},
				},
			},
			expected: &api.Caffe2JobSpec{
				ReplicaSpecs: []*api.TFReplicaSpec{
					{
						Replicas: proto.Int32(2),
						TFPort:   proto.Int32(10),
						Template: &v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name: "caffe2",
										Resources: v1.ResourceRequirements{
											Requests: map[v1.ResourceName]resource.Quantity{
												"nvidia-gpu": resource.MustParse("1"),
											},
										},
										VolumeMounts: []v1.VolumeMount{
											{
												Name:      "cuda-lib",
												MountPath: "/usr/local/cuda",
											},
										},
									},
								},
								Volumes: []v1.Volume{
									{
										Name: "cuda-lib",
										VolumeSource: v1.VolumeSource{
											HostPath: &v1.HostPathVolumeSource{
												Path: "/home/cuda",
											},
										},
									},
								},
							},
						},
						TFReplicaType: api.PS,
					},
				},
			},
			config: map[string]api.AcceleratorConfig{
				"nvidia-gpu": api.AcceleratorConfig{
					Volumes: []api.AcceleratorVolume{
						{
							Name:      "cuda-lib",
							HostPath:  "/home/cuda",
							MountPath: "/usr/local/cuda",
						},
					},
				},
			},
		},
		// Case 2 checks that we look at limit.
		{
			in: &api.Caffe2JobSpec{
				ReplicaSpecs: []*api.TFReplicaSpec{
					{
						Replicas: proto.Int32(2),
						TFPort:   proto.Int32(10),
						Template: &v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name: "caffe2",
										Resources: v1.ResourceRequirements{
											Limits: map[v1.ResourceName]resource.Quantity{
												"nvidia-gpu": resource.MustParse("1"),
											},
										},
									},
								},
							},
						},
						TFReplicaType: api.PS,
					},
				},
			},
			expected: &api.Caffe2JobSpec{
				ReplicaSpecs: []*api.TFReplicaSpec{
					{
						Replicas: proto.Int32(2),
						TFPort:   proto.Int32(10),
						Template: &v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name: "caffe2",
										Resources: v1.ResourceRequirements{
											Limits: map[v1.ResourceName]resource.Quantity{
												"nvidia-gpu": resource.MustParse("1"),
											},
										},
										VolumeMounts: []v1.VolumeMount{
											{
												Name:      "cuda-lib",
												MountPath: "/usr/local/cuda",
											},
										},
									},
								},
								Volumes: []v1.Volume{
									{
										Name: "cuda-lib",
										VolumeSource: v1.VolumeSource{
											HostPath: &v1.HostPathVolumeSource{
												Path: "/home/cuda",
											},
										},
									},
								},
							},
						},
						TFReplicaType: api.PS,
					},
				},
			},
			config: map[string]api.AcceleratorConfig{
				"nvidia-gpu": api.AcceleratorConfig{
					Volumes: []api.AcceleratorVolume{
						{
							Name:      "cuda-lib",
							HostPath:  "/home/cuda",
							MountPath: "/usr/local/cuda",
						},
					},
				},
			},
		},
		// Case 3 no GPUs
		{
			in: &api.Caffe2JobSpec{
				ReplicaSpecs: []*api.TFReplicaSpec{
					{
						Replicas: proto.Int32(2),
						TFPort:   proto.Int32(10),
						Template: &v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name: "caffe2",
									},
								},
							},
						},
						TFReplicaType: api.PS,
					},
				},
			},
			expected: &api.Caffe2JobSpec{
				ReplicaSpecs: []*api.TFReplicaSpec{
					{
						Replicas: proto.Int32(2),
						TFPort:   proto.Int32(10),
						Template: &v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name: "caffe2",
									},
								},
							},
						},
						TFReplicaType: api.PS,
					},
				},
			},
			config: map[string]api.AcceleratorConfig{
				"nvidia-gpu": api.AcceleratorConfig{
					Volumes: []api.AcceleratorVolume{
						{
							Name:      "cuda-lib",
							HostPath:  "/home/cuda",
							MountPath: "/usr/local/cuda",
						},
					},
				},
			},
		},
	}

	for _, c := range testCases {
		if err := ConfigureAcceleratorsForCaffe2JobSpec(c.in, c.config); err != nil {
			t.Errorf("ConfigureAccelerators error; %v", err)
		}
		if !reflect.DeepEqual(c.in, c.expected) {
			t.Errorf("Want\n%v; Got\n %v", util.Pformat(c.expected), util.Pformat(c.in))
		}
	}
}
