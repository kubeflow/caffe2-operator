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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	api "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
)

var (
	groupVersionKind = schema.GroupVersionKind{
		Group:   api.GroupName,
		Version: api.GroupVersion,
		Kind:    api.Caffe2JobResourceKind,
	}
)

// AsOwner make OwnerReference according to the parameter
func AsOwner(job *api.Caffe2Job) metav1.OwnerReference {
	trueVar := true
	// Both api.OwnerReference and metatypes.OwnerReference are combined into that.
	return metav1.OwnerReference{
		APIVersion:         groupVersionKind.GroupVersion().String(),
		Kind:               groupVersionKind.Kind,
		Name:               job.ObjectMeta.Name,
		UID:                job.ObjectMeta.UID,
		Controller:         &trueVar,
		BlockOwnerDeletion: &trueVar,
	}
}

// Cleanup cleans up user passed spec, e.g. defaulting, transforming fields.
// TODO: move this to admission controller
func Cleanup(c *api.Caffe2JobSpec) {
	// TODO(jlewi): Add logic to cleanup user provided spec; e.g. by filling in defaults.
	// We should have default container images so user doesn't have to provide these.
}

func CRDName() string {
	return fmt.Sprintf("%s.%s", api.CRDKindPlural, api.CRDGroup)
}

func scalingReason(from, to int) string {
	return fmt.Sprintf("Current cluster size: %d, desired cluster size: %d", from, to)
}
