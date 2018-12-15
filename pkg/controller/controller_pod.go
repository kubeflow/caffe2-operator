// Package controller provides a Kubernetes controller for a Caffe2Job resource.
package controller

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	api "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
)

// reconcilePods checks and updates pods for each given Caffe2ReplicaSpec.
// It will requeue the caffe2job in case of an error while creating/deleting pods.
func (tc *Controller) reconcilePods(
	job *api.Caffe2Job,
	pods []*v1.Pod,
	spec *api.Caffe2ReplicaSpec) error {

	// Convert Caffe2ReplicaType to lower string.
	rt := "worker"
	// Get all pods for the type rt.
	pods = filterPodsForCaffe2ReplicaType(pods, rt)
	replicas := int(*spec.Replicas)

	initializeCaffe2ReplicaStatuses(job)

	glog.Infof("Reconcile Pods...")
	podSlices := getPodSlices(pods, replicas)
	for index, podSlice := range podSlices {
		if len(podSlice) > 1 {
			glog.Warningf("We have too many pods for the worker %d", index)
			// TODO: Kill some pods.
		} else if len(podSlice) == 0 {
			glog.Infof("need to create new pod: %s-%d", rt, index)
			err := tc.createNewPod(job, rt, strconv.Itoa(index), spec)
			if err != nil {
				return err
			}
			increaseCaffe2JobReplicaStatusesActive(job)
		} else {
			// We already have one, and check the status.
			pod := podSlice[0]
			updateCaffe2JobReplicaStatuses(job, pod)
		}
	}

	return tc.updateStatus(job, replicas)
}

// getPodSlices returns a slice, which element is the slice of pod.
func getPodSlices(pods []*v1.Pod, replicas int) [][]*v1.Pod {
	podSlices := make([][]*v1.Pod, replicas)
	for _, pod := range pods {
		if _, ok := pod.Labels[caffe2ReplicaIndexLabel]; !ok {
			glog.Warning("The pod do not have the index label.")
			continue
		}
		index, err := strconv.Atoi(pod.Labels[caffe2ReplicaIndexLabel])
		if err != nil {
			glog.Warningf("Error when strconv.Atoi: %v", err)
			continue
		}
		if index < 0 || index >= replicas {
			glog.Warningf("The label index is not expected: %d", index)
		} else {
			podSlices[index] = append(podSlices[index], pod)
		}
	}
	return podSlices
}

// createNewPod creates a new pod for the given index and type.
func (tc *Controller) createNewPod(job *api.Caffe2Job, rt, index string, spec *api.Caffe2ReplicaSpec) error {
	jobKey, err := keyFunc(job)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for caffe2job object %#v: %v", job, err))
		return err
	}
	/*
		expectationPodsKey := genExpectationPodsKey(jobKey, rt)
		err = tc.expectations.ExpectCreations(expectationPodsKey, 1)
		if err != nil {
			return err
		}
	*/

	// Create OwnerReference.
	controllerRef := genOwnerReference(job)

	// Set type and index for the worker.
	labels := genLabels(job.Spec.RuntimeID, jobKey)
	labels[caffe2ReplicaTypeLabel] = rt
	labels[caffe2ReplicaIndexLabel] = index

	podTemplate := spec.Template.DeepCopy()

	if podTemplate.Labels == nil {
		podTemplate.Labels = make(map[string]string)
	}

	for key, value := range labels {
		podTemplate.Labels[key] = value
	}
	for i := range podTemplate.Spec.Containers {
		if podTemplate.Spec.Containers[i].Env == nil {
			podTemplate.Spec.Containers[i].Env = []v1.EnvVar{}
		}
		podTemplate.Spec.Containers[i].Env = append(
			podTemplate.Spec.Containers[i].Env,
			v1.EnvVar{Name: "REDIS_HOST", Value: job.Spec.Backend.RedisHost},
			v1.EnvVar{Name: "REDIS_PORT", Value: strconv.Itoa(int(job.Spec.Backend.RedisPort))},
			v1.EnvVar{Name: "NFS_PATH", Value: job.Spec.Backend.NFSPath},
			v1.EnvVar{Name: "SHARD_ID", Value: index},
			v1.EnvVar{Name: "NUM_SHARDS", Value: fmt.Sprintf("%d", *spec.Replicas)},
			v1.EnvVar{Name: "RUN_ID", Value: job.Spec.RuntimeID},
		)
	}

	// Generate Caffe2_CONFIG JSON string.
	caffe2ConfigStr := genCaffe2ConfigJSONStr(job, rt, index)

	if caffe2ConfigStr == "" {
		return nil
	}
	// Add Caffe2_CONFIG environment variable.
	for i := range podTemplate.Spec.Containers {
		if len(podTemplate.Spec.Containers[i].Env) == 0 {
			podTemplate.Spec.Containers[i].Env = make([]v1.EnvVar, 0)
		}
		podTemplate.Spec.Containers[i].Env = append(podTemplate.Spec.Containers[i].Env, v1.EnvVar{
			Name:  "CAFFE2_CONFIG",
			Value: caffe2ConfigStr,
		})
	}

	err = tc.podControl.CreatePodsWithControllerRef(job.Namespace, podTemplate, job, controllerRef)
	if err != nil && errors.IsTimeout(err) {
		// Pod is created but its initialization has timed out.
		// If the initialization is successful eventually, the
		// controller will observe the creation via the informer.
		// If the initialization fails, or if the pod keeps
		// uninitialized for a long time, the informer will not
		// receive any update, and the controller will create a new
		// pod when the expectation expires.
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

// getPodsForCaffe2Job returns the set of pods that this caffe2job should manage.
// It also reconciles ControllerRef by adopting/orphaning.
// Note that the returned Pods are pointers into the cache.
func (tc *Controller) getPodsForCaffe2Job(job *api.Caffe2Job) ([]*v1.Pod, error) {
	jobKey, err := keyFunc(job)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for caffe2job object %#v: %v", job, err))
		return nil, err
	}

	// Create selector.
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: genLabels(job.Spec.RuntimeID, jobKey),
	})

	if err != nil {
		return nil, fmt.Errorf("couldn't convert Job selector: %v", err)
	}
	// List all pods to include those that don't match the selector anymore
	// but have a ControllerRef pointing to this controller.
	pods, err := tc.podLister.Pods(job.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	// If any adoptions are attempted, we should first recheck for deletion
	// with an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh, err := tc.caffe2JobClient.KubeflowV1alpha1().Caffe2Jobs(job.Namespace).Get(job.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != job.UID {
			return nil, fmt.Errorf("original Caffe2Job %v/%v is gone: got uid %v, wanted %v", job.Namespace, job.Name, fresh.UID, job.UID)
		}
		return fresh, nil
	})
	cm := NewPodControllerRefManager(tc.podControl, job, selector, controllerKind, canAdoptFunc)
	return cm.ClaimPods(pods)
}

// filterPodsForCaffe2ReplicaType returns pods belong to a Caffe2ReplicaType.
func filterPodsForCaffe2ReplicaType(pods []*v1.Pod, caffe2ReplicaType string) []*v1.Pod {
	var result []*v1.Pod

	caffe2ReplicaSelector := &metav1.LabelSelector{
		MatchLabels: make(map[string]string),
	}

	caffe2ReplicaSelector.MatchLabels[caffe2ReplicaTypeLabel] = caffe2ReplicaType

	for _, pod := range pods {
		selector, _ := metav1.LabelSelectorAsSelector(caffe2ReplicaSelector)
		if !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		result = append(result, pod)
	}
	return result
}

func genExpectationPodsKey(jobKey, replicaType string) string {
	return jobKey + "/" + strings.ToLower(replicaType) + "/pods"
}

// When a pod is created, enqueue the caffe2job that manages it and update its expectations.
func (tc *Controller) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	if pod.DeletionTimestamp != nil {
		// on a restart of the controller controller, it's possible a new pod shows up in a state that
		// is already pending deletion. Prevent the pod from being a creation observation.
		// tc.deletePod(pod)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil {
		job := tc.resolveControllerRef(pod.Namespace, controllerRef)
		if job == nil {
			glog.Info("This pod's caffe2job does not exists")
			return
		}

		/*
			jobKey, err := keyFunc(job)
			if err != nil {
				glog.Infof("Failed to get the key of the caffe2job: %v", err)
				return
			}
		*/

		if _, ok := pod.Labels[caffe2ReplicaTypeLabel]; !ok {
			glog.Info("This pod maybe not created by caffe2-operator")
			return
		}

		/* TODO
		rtype := pod.Labels[caffe2ReplicaTypeLabel]
		expectationPodsKey := genExpectationPodsKey(jobKey, rtype)
		tc.expectations.CreationObserved(expectationPodsKey)
		*/
		tc.enqueueController(job)

		return
	}

	// Otherwise, it's an orphan. Get a list of all matching controllers and sync
	// them to see if anyone wants to adopt it.
	// DO NOT observe creation because no controller should be waiting for an
	// orphan.
	// for _, job := range tc.getPodJobs(pod) {
	// 	tc.enqueueCaffe2Job(job)
	// }
}

// When a pod is updated, figure out what job/s manage it and wake them up.
// If the labels of the pod have changed we need to awaken both the old
// and new replica set. old and cur must be *v1.Pod types.
func (tc *Controller) updatePod(old, cur interface{}) {
	// TODO: handle this gracefully.
}

// When a pod is deleted, enqueue the caffe2job that manages the pod and update its expectations.
// obj could be an *v1.Pod, or a DeletionFinalStateUnknown marker item.
func (tc *Controller) deletePod(obj interface{}) {
	// TODO: handle this gracefully.
}
