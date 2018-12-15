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

// Package controller provides a Kubernetes controller for a Caffe2Job resource.
package controller

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	log "github.com/golang/glog"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	api "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
)

// Caffe2Config is a struct representing the distributed TensorFlow config.
// This struct is turned into an environment variable CAFFE2_CONFIG
// which is used by TensorFlow processes to configure themselves.
// https://cloud.google.com/ml-engine/docs/trainer-considerations#use_tf_config
type Caffe2Config struct {
	// Cluster represents a TensorFlow ClusterSpec.
	// See: https://www.tensorflow.org/api_docs/python/tf/train/ClusterSpec
	Cluster ClusterSpec `json:"cluster"`
	Task    TaskSpec    `json:"task"`
}

// ClusterSpec represents a cluster Caffe2 specification.
// https://www.tensorflow.org/deploy/distributed#create_a_tftrainclusterspec_to_describe_the_cluster
// It is a map from job names to network addresses.
type ClusterSpec map[string][]string

type TaskSpec struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

// genCaffe2Config will generate the environment variable Caffe2_CONFIG
// {
//     "cluster": {
//         "worker": ["worker1:2222", "worker2:2222", "worker3:2222"]
//     },
//     "task": {
//         "type": "ps",
//         "index": 1
//         },
//     }
// }
func genCaffe2ConfigJSONStr(job *api.Caffe2Job, rtype, index string) string {
	// Configure the CAFFE2_CONFIG environment variable.
	i, _ := strconv.ParseInt(index, 0, 32)

	config := Caffe2Config{
		Cluster: genClusterSpec(job),
		Task: TaskSpec{
			Type:  rtype,
			Index: int(i),
		},
	}

	configJSONStr, err := json.Marshal(config)
	if err != nil {
		log.Errorf("Caffe2Job: %v serializing config return error: %v", job.Name, err)
		return ""
	}

	return string(configJSONStr)
}

// genClusterSpec will generate ClusterSpec.
func genClusterSpec(job *api.Caffe2Job) ClusterSpec {
	jobKey, err := keyFunc(job)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for caffe2job object %#v: %v", job, err))
		return nil
	}

	clusterSpec := make(ClusterSpec)

	rtype := "worker"
	spec := job.Spec.ReplicaSpecs
	replicaNames := make([]string, 0, *spec.Replicas)

	for i := int32(0); i < *spec.Replicas; i++ {
		host := genGeneralName(jobKey, rtype, fmt.Sprintf("%d", i)) + ":" + strconv.Itoa(api.Caffe2Port)
		replicaNames = append(replicaNames, host)
	}

	clusterSpec[rtype] = replicaNames

	return clusterSpec
}

func genGeneralName(jobKey, rtype, index string) string {
	n := jobKey + "-" + rtype + "-" + index
	return strings.Replace(n, "/", "-", -1)
}
