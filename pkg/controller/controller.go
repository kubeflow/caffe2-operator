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

// Package controller provides a Kubernetes controller for a Caffe2 job resource.
package controller

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sinformers "k8s.io/client-go/informers"
	clientv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	api "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
	jobclient "github.com/kubeflow/caffe2-operator/pkg/client/clientset/versioned"
	kubeflowscheme "github.com/kubeflow/caffe2-operator/pkg/client/clientset/versioned/scheme"
	informers "github.com/kubeflow/caffe2-operator/pkg/client/informers/externalversions"
	"github.com/kubeflow/caffe2-operator/pkg/client/informers/externalversions/kubeflow/v1alpha1"
	listers "github.com/kubeflow/caffe2-operator/pkg/client/listers/kubeflow/v1alpha1"
)

const (
	controllerName = "caffe2-operator"

	// labels for pods and servers.
	caffe2ReplicaTypeLabel  = "caffe2_replica_type"
	caffe2ReplicaIndexLabel = "caffe2_replica_index"
)

var (
	ErrVersionOutdated = errors.New("requested version is outdated in apiserver")

	// IndexerInformer uses a delta queue, therefore for deletes we have to use this
	// key function but it should be just fine for non delete events.
	keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

	// DefaultJobBackOff is the max backoff period, exported for the e2e test
	DefaultJobBackOff = 10 * time.Second
	// MaxJobBackOff is the max backoff period, exported for the e2e test
	MaxJobBackOff = 360 * time.Second
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = api.SchemeGroupVersion.WithKind("Caffe2Job")

var groupVersionKind = schema.GroupVersionKind{
	Group:   api.GroupName,
	Version: api.GroupVersion,
	Kind:    api.Caffe2JobResourceKind,
}

type ControllerConfiguration struct {
	ReconcilerSyncLoopPeriod metav1.Duration
}

// DefaultCaffe2JobControllerConfiguration is the suggested caffe2-operator configuration for production.
var DefaultCaffe2JobControllerConfiguration ControllerConfiguration = ControllerConfiguration{
	ReconcilerSyncLoopPeriod: metav1.Duration{Duration: 15 * time.Second},
}

type Controller struct {
	config ControllerConfiguration

	// podControl is used to add or delete pods.
	podControl PodControlInterface
	// serviceControl is used to add or delete services.
	serviceControl ServiceControlInterface
	// kubeClient is a standard kubernetes clientset.
	kubeClient kubernetes.Interface
	// caffe2JobClientSet is a clientset for CRD Caffe2Job.
	caffe2JobClient jobclient.Interface

	// caffe2JobLister can list/get caffe2jobs from the shared informer's store.
	caffe2JobLister listers.Caffe2JobLister
	// podLister can list/get pods from the shared informer's store.
	podLister corelisters.PodLister
	// serviceLister can list/get services from the shared informer's store.
	serviceLister corelisters.ServiceLister

	podInformer       clientv1.PodInformer
	caffe2JobInformer v1alpha1.Caffe2JobInformer

	// returns true if the caffe2job store has been synced at least once.
	caffe2JobSynced cache.InformerSynced
	// podListerSynced returns true if the pod store has been synced at least once.
	podListerSynced cache.InformerSynced
	// serviceListerSynced returns true if the service store has been synced at least once.
	serviceListerSynced cache.InformerSynced

	// WorkQueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workQueue workqueue.RateLimitingInterface

	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder

	// To allow injection of syncCaffe2Job for testing.
	syncHandler func(jobKey string) (bool, error)
	// To allow injection of updateStatus for testing.
	updateStatusHandler func(job *api.Caffe2Job) error
}

func New(kubeClient kubernetes.Interface, caffe2JobClient jobclient.Interface) (*Controller, error) {
	kubeflowscheme.AddToScheme(scheme.Scheme)
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: controllerName})

	podControl := RealPodControl{
		KubeClient: kubeClient,
		Recorder:   eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "caffe2job-controller"}),
	}
	serviceControl := RealServiceControl{
		KubeClient: kubeClient,
		Recorder:   eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "caffe2job-controller"}),
	}

	controller := &Controller{
		podControl:      podControl,
		serviceControl:  serviceControl,
		kubeClient:      kubeClient,
		caffe2JobClient: caffe2JobClient,
		workQueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Caffe2jobs"),
		recorder:        recorder,
	}
	caffe2JobInformerFactory := informers.NewSharedInformerFactory(caffe2JobClient, time.Second*30)
	podInformerFactory := k8sinformers.NewSharedInformerFactory(kubeClient, time.Second*30)

	controller.caffe2JobInformer = caffe2JobInformerFactory.Kubeflow().V1alpha1().Caffe2Jobs()

	glog.Info("Setting up event handlers")
	// Set up an event handler for when Foo resources change
	controller.caffe2JobInformer.Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *api.Caffe2Job:
					glog.V(4).Infof("filter caffe2job name: %v", t.Name)
					return true
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    controller.addCaffe2Job,
				UpdateFunc: controller.updateCaffe2Job,
				DeleteFunc: controller.enqueueController,
			},
		})
	controller.caffe2JobLister = controller.caffe2JobInformer.Lister()
	controller.caffe2JobSynced = controller.caffe2JobInformer.Informer().HasSynced

	// create informer for pod information
	controller.podInformer = podInformerFactory.Core().V1().Pods()
	controller.podInformer.Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch obj.(type) {
				case *v1.Pod:
					pod := obj.(*v1.Pod)
					if _, ok := pod.Labels["caffe2_job_key"]; !ok {
						return false
					}
					return pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    controller.addPod,
				UpdateFunc: controller.updatePod,
				DeleteFunc: controller.deletePod,
			},
		})
	controller.podLister = controller.podInformer.Lister()
	controller.podListerSynced = controller.podInformer.Informer().HasSynced

	controller.syncHandler = controller.syncCaffe2Job
	controller.updateStatusHandler = controller.updateCaffe2JobStatus

	return controller, nil
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workQueue.ShutDown()

	go c.podInformer.Informer().Run(stopCh)
	go c.caffe2JobInformer.Informer().Run(stopCh)

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting Caffe2Job controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	glog.V(4).Info("Sync caffe2jobs...")
	if ok := cache.WaitForCacheSync(stopCh, c.caffe2JobSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	glog.V(4).Info("Sync pods...")
	if ok := cache.WaitForCacheSync(stopCh, c.podListerSynced); !ok {
		return fmt.Errorf("failed to wait for pod caches to sync")
	}

	glog.Infof("Starting %v workers", threadiness)
	// Launch workers to process Caffe2Job resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	key, quit := c.workQueue.Get()
	if quit {
		return false
	}
	defer c.workQueue.Done(key)

	forget, err := c.syncHandler(key.(string))
	if err == nil {
		if forget {
			c.workQueue.Forget(key)
		}
		return true
	}

	utilruntime.HandleError(fmt.Errorf("Error syncing job: %v", err))
	c.workQueue.AddRateLimited(key)

	return true
}

// syncCaffe2Job will sync the job with the given. This function is not meant to be invoked
// concurrently with the same key.
//
// When a job is completely processed it will return true indicating that its ok to forget about this job since
// no more processing will occur for it.
func (c *Controller) syncCaffe2Job(key string) (bool, error) {
	startTime := time.Now()
	defer func() {
		glog.V(4).Infof("Finished syncing job %q (%v)", key, time.Since(startTime))
	}()

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return false, err
	}
	if len(ns) == 0 || len(name) == 0 {
		return false, fmt.Errorf("invalid job key %q: either namespace or name is missing", key)
	}

	job, err := c.caffe2JobLister.Caffe2Jobs(ns).Get(name)

	if err != nil {
		if apierrors.IsNotFound(err) {
			glog.V(4).Infof("Job has been deleted: %v", key)
			return true, nil
		}
		return false, err
	}

	glog.Infof("Caffe2Jobs: %#v", job)

	var reconcileCaffe2JobsErr error
	if job.DeletionTimestamp == nil {
		reconcileCaffe2JobsErr = c.reconcileCaffe2Jobs(job)
	}
	if reconcileCaffe2JobsErr != nil {
		return false, reconcileCaffe2JobsErr
	}

	return true, err
}

// obj could be an *batch.Job, or a DeletionFinalStateUnknown marker item.
func (c *Controller) enqueueController(obj interface{}) {
	key, err := keyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for object %+v: %v", obj, err))
		return
	}

	c.workQueue.AddRateLimited(key)
}

// reconcileCaffe2Jobs checks and updates replicas for each given Caffe2ReplicaSpec.
// It will requeue the caffe2job in case of an error while creating/deleting pods/services.
func (c *Controller) reconcileCaffe2Jobs(job *api.Caffe2Job) error {
	glog.Infof("Reconcile Caffe2Jobs %s", job.Name)

	pods, err := c.getPodsForCaffe2Job(job)

	if err != nil {
		glog.Infof("getPodsForCaffe2Job error %v", err)
		return err
	}

	glog.V(4).Infof("Pods is %#v", pods)

	/* TODO services
	services, err := c.getServicesForCaffe2Job(job)

	if err != nil {
		glog.Infof("getServicesForCaffe2Job error %v", err)
		return err
	}
	*/

	// Diff current active pods/services with replicas.
	spec := job.Spec.ReplicaSpecs
	err = c.reconcilePods(job, pods, spec)
	if err != nil {
		glog.Infof("reconcilePods error %v", err)
		return err
	}

	/*
		err = c.reconcileServices(job, services, rtype, spec)

		if err != nil {
			glog.Infof("reconcileServices error %v", err)
			return err
		}
	*/

	// TODO: Add check here, no need to update the caffe2job if the status hasn't changed since last time.
	return c.updateStatusHandler(job)
}

func genLabels(id, jobKey string) map[string]string {
	return map[string]string{
		"group_name":     api.GroupName,
		"caffe2_job_key": strings.Replace(jobKey, "/", "-", -1),
		"runtime_id":     id,
	}
}

// When a pod is added, set the defaults and enqueue the current caffe2job.
func (c *Controller) addCaffe2Job(obj interface{}) {
	job := obj.(*api.Caffe2Job)
	msg := fmt.Sprintf("Caffe2Job %s is created.", job.Name)
	glog.Info(msg)
	scheme.Scheme.Default(job)

	// Leave a created condition.
	err := c.updateCaffe2JobConditions(job, api.Caffe2JobCreated, caffe2JobCreatedReason, msg)
	if err != nil {
		glog.Errorf("Append caffe2job condition error: %v", err)
		return
	}

	c.enqueueController(obj)
}

// When a pod is updated, enqueue the current caffe2job.
func (c *Controller) updateCaffe2Job(old, cur interface{}) {
	oldCaffe2Job := old.(*api.Caffe2Job)
	glog.Infof("Updating caffe2job: %s", oldCaffe2Job.Name)
	c.enqueueController(cur)
}

func (c *Controller) updateCaffe2JobStatus(job *api.Caffe2Job) error {
	_, err := c.caffe2JobClient.KubeflowV1alpha1().Caffe2Jobs(job.Namespace).Update(job)
	return err
}

func (c *Controller) updateCaffe2JobConditions(job *api.Caffe2Job, conditionType api.Caffe2JobConditionType, reason, message string) error {
	condition := newCondition(conditionType, reason, message)
	setCondition(&job.Status, condition)
	return nil
}

// resolveControllerRef returns the tfjob referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching tfjob
// of the correct Kind.
func (c *Controller) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *api.Caffe2Job {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}
	job, err := c.caffe2JobLister.Caffe2Jobs(namespace).Get(controllerRef.Name)
	if err != nil {
		return nil
	}
	if job.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return job
}

func genOwnerReference(job *api.Caffe2Job) *metav1.OwnerReference {
	boolPtr := func(b bool) *bool { return &b }
	controllerRef := &metav1.OwnerReference{
		APIVersion:         groupVersionKind.GroupVersion().String(),
		Kind:               groupVersionKind.Kind,
		Name:               job.Name,
		UID:                job.UID,
		BlockOwnerDeletion: boolPtr(true),
		Controller:         boolPtr(true),
	}

	return controllerRef
}

// newCondition creates a new caffe2job condition.
func newCondition(conditionType api.Caffe2JobConditionType, reason, message string) api.Caffe2JobCondition {
	return api.Caffe2JobCondition{
		Type:               conditionType,
		Status:             v1.ConditionTrue,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// getCondition returns the condition with the provided type.
func getCondition(status api.Caffe2JobStatus, condType api.Caffe2JobConditionType) *api.Caffe2JobCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// setCondition updates the caffe2job to include the provided condition.
// If the condition that we are about to add already exists
// and has the same status and reason then we are not going to update.
func setCondition(status *api.Caffe2JobStatus, condition api.Caffe2JobCondition) {
	currentCond := getCondition(*status, condition.Type)

	// Do nothing if condition doesn't change
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	// Do not update lastTransitionTime if the status of the condition doesn't change.
	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}

	// Append the updated condition to the
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, condition)
}

// removeCondition removes the caffe2job condition with the provided type.
func removementCondition(status *api.Caffe2JobStatus, condType api.Caffe2JobConditionType) {
	status.Conditions = filterOutCondition(status.Conditions, condType)
}

// filterOutCondition returns a new slice of caffe2job conditions without conditions with the provided type.
func filterOutCondition(conditions []api.Caffe2JobCondition, condType api.Caffe2JobConditionType) []api.Caffe2JobCondition {
	var newConditions []api.Caffe2JobCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
