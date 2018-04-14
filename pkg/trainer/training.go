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

// Package trainer is to manage Caffe2 training jobs.
package trainer

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/helper"
	api "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
	"github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/validation"
	jobclient "github.com/kubeflow/caffe2-operator/pkg/client/clientset/versioned"
	"github.com/kubeflow/caffe2-operator/pkg/client/clientset/versioned/scheme"
	"github.com/kubeflow/caffe2-operator/pkg/util"

	log "github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
)

// TODO: We should switch a New pattern and make trainingJob private so we can
// ensure correctness on creation.
type TrainingJob struct {
	job *api.Caffe2Job

	KubeCli kubernetes.Interface

	recorder record.EventRecorder

	Replicas []*Caffe2ReplicaSet

	jobClient jobclient.Interface

	// in memory state of the job.
	// status is the source of truth after job struct is materialized. Changes to the status to be persisted
	// should be made here.
	status api.Caffe2JobStatus

	memberCounter int
}

// ClusterSpec represents a cluster Caffe2 specification.
// https://www.tensorflow.org/deploy/distributed#create_a_tftrainclusterspec_to_describe_the_cluster
// It is a map from job names to network addresses.
type ClusterSpec map[string][]string

type TaskSpec struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

func initJob(kubeCli kubernetes.Interface, jobClient jobclient.Interface, recorder record.EventRecorder, job *api.Caffe2Job) (*TrainingJob, error) {
	j := &TrainingJob{
		KubeCli:   kubeCli,
		jobClient: jobClient,
		recorder:  recorder,
		Replicas:  make([]*Caffe2ReplicaSet, 0),
		job:       job,
		status:    *job.Status.DeepCopy(),
	}

	return j, nil
}

func NewJob(kubeCli kubernetes.Interface, jobClient jobclient.Interface, recorder record.EventRecorder, job *api.Caffe2Job, config *api.ControllerConfig) (*TrainingJob, error) {
	j, err := initJob(kubeCli, jobClient, recorder, job)
	if err != nil {
		return nil, err
	}

	return j, nil
}

func (j *TrainingJob) UID() types.UID {
	return j.job.ObjectMeta.UID
}

func (j *TrainingJob) ClusterSpec() ClusterSpec {
	clusterSpec := make(ClusterSpec)

	for _, p := range j.Replicas {
		replicaNames := make([]string, 0, *p.Spec.Replicas)

		for i := int32(0); i < *p.Spec.Replicas; i++ {
			replicaNames = append(replicaNames, fmt.Sprintf("%v:%v", p.jobName(i), *p.Spec.Caffe2Port))
		}

		clusterSpec[strings.ToLower(string(p.Spec.Caffe2ReplicaType))] = replicaNames
	}

	return clusterSpec
}

// createResources creates all the replicas if requested
func (j *TrainingJob) createResources(config *api.ControllerConfig) error {
	for _, r := range j.Replicas {
		if err := r.Create(config); err != nil {
			return err
		}
	}

	return nil
}

// deleteResources deletes the replicas it it was created
func (j *TrainingJob) deleteResources() error {
	for _, r := range j.Replicas {
		if err := r.Delete(); err != nil {
			return err
		}
	}

	return nil
}

func (j *TrainingJob) GetStatus() (api.State, []*api.Caffe2ReplicaStatus, error) {
	chief := j.job.Spec.TerminationPolicy.Chief
	chiefState := api.ReplicaStateUnknown

	state := api.StateUnknown
	replicaStatuses := make([]*api.Caffe2ReplicaStatus, 0)

	// The state for each replica.
	// TODO: We will need to modify this code if we want to allow multiples of a given type of replica.
	replicaSetStates := make(map[api.Caffe2ReplicaType]api.ReplicaState)

	for _, r := range j.Replicas {
		rStatus, err := r.GetStatus()
		if err != nil {
			log.Errorf("GetStatus() for %v returned error; %v", r.Spec.Caffe2ReplicaType, err)
		}

		replicaSetStates[r.Spec.Caffe2ReplicaType] = rStatus.State

		replicaStatuses = append(replicaStatuses, &rStatus)

		if string(r.Spec.Caffe2ReplicaType) == chief.ReplicaName {
			chiefState = r.GetSingleReplicaStatus(int32(chief.ReplicaIndex))
		}
	}

	if chiefState == api.ReplicaStateRunning {
		state = api.StateRunning
	} else if chiefState == api.ReplicaStateFailed {
		state = api.StateFailed
	} else if chiefState == api.ReplicaStateSucceeded {
		state = api.StateSucceeded
	}

	return state, replicaStatuses, nil
}

// isRetryableTerminationState returns true if a container terminated in a state
// that we consider retryable.
func isRetryableTerminationState(s *v1.ContainerStateTerminated) bool {
	// TODO: Need to match logic in
	// https://cs.corp.google.com/piper///depot/google3/cloud/ml/beta/job/training_job_state_util.cc?l=88
	if s.Reason == "OOMKilled" {
		// If the user's process causes an OOM and Docker kills the container,
		// the termination reason of ContainerState will be specified to
		// 'OOMKilled'. In this case, we can't assume this to be a retryable error.
		//
		// This check should happen before checking the termination log, since
		// if the container terminated with an OOM, the termination log may not
		// be written.
		return false
	}

	// TODO: Should we use the exit code reported in the termination
	// log message and not the ExitCode reported by the container.

	if s.ExitCode >= 0 && s.ExitCode <= 127 {
		// For the exit_code in [0, 127]:
		//   0 means success,
		//   1 - 127 corresponds to permanent user errors.
		// We don't want to retry for both cases.
		// More info about exit status can be found in:
		// https://www.gnu.org/software/bash/manual/html_node/Exit-Status.html
		return false
	}

	// For the remaining cases that exit_code from workers that doesn't
	// fall into [0, 127]. They can be:
	//   137 corresponds to SIGKILL,
	//   143 corresponds to SIGTERM,
	//   other values that have undefined behavior.
	// We treat them as internal errors for now and all the internal errors
	// will be retired.
	return true
}

func (j *TrainingJob) masterName() string {
	return fmt.Sprintf("master-%v-0", j.job.Spec.RuntimeId)
}

// setup the training job.
func (j *TrainingJob) setup(config *api.ControllerConfig) {
	if j.job == nil {
		j.status.Reason = "Internal error; setup failed; job is missing spec."
		j.status.Phase = api.Caffe2JobPhaseFailed
		j.status.State = api.StateFailed
	}

	err := func() error {
		// If the job has already started we shouldn't set it up again.
		if j.status.Phase != api.Caffe2JobPhaseNone {
			log.Warningf("Job %v has already been setup.", j.name())
			return nil
		}

		// Set defaults.
		scheme.Scheme.Default(j.job)

		err := validation.ValidateCaffe2JobSpec(&j.job.Spec)
		if err != nil {
			return fmt.Errorf("invalid job spec: %v", err)
		}

		if err := helper.ConfigureAcceleratorsForCaffe2JobSpec(&j.job.Spec, config.Accelerators); err != nil {
			return fmt.Errorf("ConfigureAccelerators(...) error; %v", err)
		}

		if j.job.Spec.RuntimeId == "" {
			j.job.Spec.RuntimeId = util.RandString(4)
		}
		return nil
	}()

	if err != nil {
		j.status.Reason = err.Error()
		j.status.Phase = api.Caffe2JobPhaseFailed
		j.status.State = api.StateFailed
	} else {
		j.status.Phase = api.Caffe2JobPhaseCreating
		j.status.State = api.StateRunning
	}
}

// setup Replicas. This creates in memory data structures corresponding to the replicas.
func (j *TrainingJob) setupReplicas() error {

	if len(j.Replicas) != len(j.job.Spec.ReplicaSpecs) {
		j.Replicas = make([]*Caffe2ReplicaSet, 0, len(j.job.Spec.ReplicaSpecs))
		for _, t := range j.job.Spec.ReplicaSpecs {
			r, err := NewCaffe2ReplicaSet(j.KubeCli, j.recorder, *t, j)
			if err != nil {
				return err
			}
			j.Replicas = append(j.Replicas, r)
		}
	}

	return nil
}

func (j *TrainingJob) Delete() {
	// TODO: Delete is what should cause us to delete the Pods.
	// we shouldn't delete the pods when the jobs finish because leaving the pods
	// allows us to get the logs from the pods after the job finishes.
	//
	log.Infof("Caffe2Job %v deleted by the user", j.fullname())
	// TODO: This logic is probably insufficient.
	if j.job.Status.Phase != api.Caffe2JobPhaseCleanUp {
		j.status.Phase = api.Caffe2JobPhaseCleanUp
	}

	// TODO: Does it make sense to explicitly delete the resources? Should
	// we just rely on K8s garbage collection to delete the resources before
	// deleting TFJob?
	if cErr := j.deleteResources(); cErr != nil {
		log.Errorf("trainingJob.deleteResources() error; %v", cErr)
	}
}

// updateCRDStatus updates the job status based on TraingingJob.status.
func (j *TrainingJob) updateCRDStatus() error {
	// If the status hasn't changed then there's no reason to update the CRD.
	if reflect.DeepEqual(j.job.Status, j.status) {
		return nil
	}

	newJob := j.job
	newJob.Status = j.status
	newJob, err := j.jobClient.KubeflowV1alpha1().Caffe2Jobs(j.job.ObjectMeta.Namespace).Update(newJob)
	if err != nil {
		return err
	}

	j.job = newJob

	return nil
}

// reconcile tries to get the job into the desired state.
func (j *TrainingJob) Reconcile(config *api.ControllerConfig) error {
	if j.job.Status.Phase == api.Caffe2JobPhaseNone {
		// The job hasn't been setup.
		j.setup(config)

		if err := j.updateCRDStatus(); err != nil {
			log.Warningf("failed to update CRD status: %v", err)
			return err
		}
	}

	// setupreplicas initializes data structures inside TrainingJob representing the replicas.
	// These are go-lang structures which aren't preserved in the APIServer. So we always need to call setupReplicas
	// unlike setup which only needs to be called once during the lifecycle of the job.
	if err := j.setupReplicas(); err != nil {
		log.Errorf("failed to create replicas: %v", err)
		j.status.Reason = fmt.Sprintf("Could not create in memory datastructures; %v", err)
		if uErr := j.updateCRDStatus(); err != nil {
			log.Warningf("Job %v; failed to update status error: %v", j.job.ObjectMeta.Name, uErr)
		}
		return err
	}

	// TODO: Can we determine from the CRD status whether we should
	// Create the resources or not? We need to ensure the resources exist so for
	// now we always call Create.
	if j.job.Status.Phase == api.Caffe2JobPhaseCreating || j.job.Status.Phase == api.Caffe2JobPhaseRunning {
		// We call Create to make sure all the resources exist and are running.
		if cErr := j.createResources(config); cErr != nil {
			// TODO: Should we eventually give up and mark the job as failed if we can't create the resources?
			j.status.Reason = fmt.Sprintf("Could not create job resources; %v", cErr)
			if err := j.updateCRDStatus(); err != nil {
				log.Warningf("Job %v; failed to update status error: %v", j.job.ObjectMeta.Name, err)
				return err
			}
			log.Errorf("trainingJobCreateReplicas() error; %v", cErr)
			return cErr
		}

		state, replicaStatuses, err := j.GetStatus()

		j.status.ReplicaStatuses = replicaStatuses
		if err != nil {
			log.Errorf("GetStatus() for job %v returned error: %v", j.job.ObjectMeta.Name, err)
			return err
		}
		// TODO: We should update the Phase if we detect the job is done.
		if state == api.StateFailed {
			log.Errorf("Master failed Job: %v.", j.job.ObjectMeta.Name)
			j.status.Phase = api.Caffe2JobPhaseDone
			j.status.State = api.StateFailed
		} else if state == api.StateSucceeded {
			log.Infof("Master succeeded Job: %v.", j.job.ObjectMeta.Name)
			j.status.Phase = api.Caffe2JobPhaseDone
			j.status.State = api.StateSucceeded
		} else {
			log.V(1).Infof("Job %v status=%v", j.job.ObjectMeta.Name, util.Pformat(j.status))
		}
	}

	// If the phase changed we should update the CRD.
	if err := j.updateCRDStatus(); err != nil {
		log.Warningf("Job %v, failed to update CRD status error: %v", j.job.ObjectMeta.Name, err)
		return err
	}

	if j.job.Status.Phase == api.Caffe2JobPhaseCleanUp {
		if cErr := j.deleteResources(); cErr != nil {
			log.Errorf("Job %v trainingJob.Delete() error; %v", j.job.ObjectMeta.Name, cErr)
		}
		// j.status.SetPhase(spec.Caffe2JobPhaseDone)
		// Return from run because we want to stop reconciling the object.
		return nil
	}

	// updateCRDStatus will update the status of the CRD with c.Status if c.Status
	// doesn't match c.Cluster.status. So you can change c.Status in order to propagate
	// changes to the CRD status.
	if err := j.updateCRDStatus(); err != nil {
		log.Warningf("Job %v; failed to update CRD status error: %v", j.job.ObjectMeta.Name, err)
		return err
	}

	return nil
}

func (j *TrainingJob) name() string {
	return j.job.ObjectMeta.GetName()
}

// fullname returns the namespace and name for the job.
func (j *TrainingJob) fullname() string {
	return j.job.ObjectMeta.GetNamespace() + ":" + j.job.ObjectMeta.GetName()
}
