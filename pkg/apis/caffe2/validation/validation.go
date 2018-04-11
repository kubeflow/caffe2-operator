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

package validation

import (
	"fmt"

	api "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
	"github.com/kubeflow/caffe2-operator/pkg/util"
)

// ValidateCaffe2JobSpec checks that the Caffe2JobSpec is valid.
func ValidateCaffe2JobSpec(c *api.Caffe2JobSpec) error {
	if c.TerminationPolicy == nil || c.TerminationPolicy.Chief == nil {
		return fmt.Errorf("invalid termination policy: %v", c.TerminationPolicy)
	}

	chiefExists := false

	// Check that each replica has a Caffe2 container and a chief.
	for _, r := range c.ReplicaSpecs {
		found := false
		if r.Template == nil {
			return fmt.Errorf("Replica is missing Template; %v", util.Pformat(r))
		}

		if r.Caffe2ReplicaType == api.Caffe2ReplicaType(c.TerminationPolicy.Chief.ReplicaName) {
			chiefExists = true
		}

		if r.Caffe2Port == nil {
			return fmt.Errorf("caffe2ReplicaSpec.Caffe2Port can't be nil.")
		}

		// Make sure the replica type is valid.
		validReplicaTypes := []api.Caffe2ReplicaType{api.MASTER, api.WORKER}

		isValidReplicaType := false
		for _, t := range validReplicaTypes {
			if t == r.Caffe2ReplicaType {
				isValidReplicaType = true
				break
			}
		}

		if !isValidReplicaType {
			return fmt.Errorf("caffe2ReplicaSpec.Caffe2ReplicaType is %v but must be one of %v", r.Caffe2ReplicaType, validReplicaTypes)
		}

		for _, c := range r.Template.Spec.Containers {
			if c.Name == string(api.CAFFE2) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Replica type %v is missing a container named %v", r.Caffe2ReplicaType, api.CAFFE2)
		}
	}

	if !chiefExists {
		return fmt.Errorf("Missing ReplicaSpec for chief: %v", c.TerminationPolicy.Chief.ReplicaName)
	}

	return nil
}
