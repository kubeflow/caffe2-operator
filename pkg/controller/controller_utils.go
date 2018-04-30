package controller

import (
	"fmt"
	"sync"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
)

// Reasons for pod events
const (
	// FailedCreatePodReason is added in an event and in a replica set condition
	// when a pod for a replica set is failed to be created.
	FailedCreatePodReason = "FailedCreate"
	// SuccessfulCreatePodReason is added in an event when a pod for a replica set
	// is successfully created.
	SuccessfulCreatePodReason = "SuccessfulCreate"
	// FailedDeletePodReason is added in an event and in a replica set condition
	// when a pod for a replica set is failed to be deleted.
	FailedDeletePodReason = "FailedDelete"
	// SuccessfulDeletePodReason is added in an event when a pod for a replica set
	// is successfully deleted.
	SuccessfulDeletePodReason = "SuccessfulDelete"

	FailedCreateServiceReason     = "FailedCreateService"
	SuccessfulCreateServiceReason = "SuccessfulCreateService"
)

// PodControlInterface is an interface that knows how to add or delete pods
// created as an interface to allow testing.
type PodControlInterface interface {
	// CreatePods creates new pods according to the spec.
	CreatePods(namespace string, template *v1.PodTemplateSpec, object runtime.Object) error
	// CreatePodsOnNode creates a new pod according to the spec on the specified node,
	// and sets the ControllerRef.
	CreatePodsOnNode(nodeName, namespace string, template *v1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error
	// CreatePodsWithControllerRef creates new pods according to the spec, and sets object as the pod's controller.
	CreatePodsWithControllerRef(namespace string, template *v1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error
	// DeletePod deletes the pod identified by podID.
	DeletePod(namespace string, podID string, object runtime.Object) error
	// PatchPod patches the pod.
	PatchPod(namespace, name string, data []byte) error
}

var _ PodControlInterface = &RealPodControl{}

// RealPodControl is the default implementation of PodControlInterface.
type RealPodControl struct {
	KubeClient clientset.Interface
	Recorder   record.EventRecorder
}

func getPodsLabelSet(template *v1.PodTemplateSpec) labels.Set {
	desiredLabels := make(labels.Set)
	for k, v := range template.Labels {
		desiredLabels[k] = v
	}
	return desiredLabels
}

func getPodsFinalizers(template *v1.PodTemplateSpec) []string {
	desiredFinalizers := make([]string, len(template.Finalizers))
	copy(desiredFinalizers, template.Finalizers)
	return desiredFinalizers
}

func getPodsAnnotationSet(template *v1.PodTemplateSpec) labels.Set {
	desiredAnnotations := make(labels.Set)
	for k, v := range template.Annotations {
		desiredAnnotations[k] = v
	}
	return desiredAnnotations
}

func validatePodName(name string, prefix bool) []string {
	return apimachineryvalidation.NameIsDNSSubdomain(name, prefix)
}

func getPodsPrefix(controllerName string) string {
	// use the dash (if the name isn't too long) to make the pod name a bit prettier
	prefix := fmt.Sprintf("%s-", controllerName)
	if len(validatePodName(prefix, true)) != 0 {
		prefix = controllerName
	}
	return prefix
}

func validateControllerRef(controllerRef *metav1.OwnerReference) error {
	if controllerRef == nil {
		return fmt.Errorf("controllerRef is nil")
	}
	if len(controllerRef.APIVersion) == 0 {
		return fmt.Errorf("controllerRef has empty APIVersion")
	}
	if len(controllerRef.Kind) == 0 {
		return fmt.Errorf("controllerRef has empty Kind")
	}
	if controllerRef.Controller == nil || *controllerRef.Controller != true {
		return fmt.Errorf("controllerRef.Controller is not set to true")
	}
	if controllerRef.BlockOwnerDeletion == nil || *controllerRef.BlockOwnerDeletion != true {
		return fmt.Errorf("controllerRef.BlockOwnerDeletion is not set")
	}
	return nil
}

func (r RealPodControl) CreatePods(namespace string, template *v1.PodTemplateSpec, object runtime.Object) error {
	return r.createPods("", namespace, template, object, nil)
}

func (r RealPodControl) CreatePodsWithControllerRef(namespace string, template *v1.PodTemplateSpec, controllerObject runtime.Object, controllerRef *metav1.OwnerReference) error {
	if err := validateControllerRef(controllerRef); err != nil {
		return err
	}
	return r.createPods("", namespace, template, controllerObject, controllerRef)
}

func (r RealPodControl) CreatePodsOnNode(nodeName, namespace string, template *v1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	if err := validateControllerRef(controllerRef); err != nil {
		return err
	}
	return r.createPods(nodeName, namespace, template, object, controllerRef)
}

func (r RealPodControl) PatchPod(namespace, name string, data []byte) error {
	_, err := r.KubeClient.CoreV1().Pods(namespace).Patch(name, types.StrategicMergePatchType, data)
	return err
}

func GetPodFromTemplate(template *v1.PodTemplateSpec, parentObject runtime.Object, controllerRef *metav1.OwnerReference) (*v1.Pod, error) {
	desiredLabels := getPodsLabelSet(template)
	desiredFinalizers := getPodsFinalizers(template)
	desiredAnnotations := getPodsAnnotationSet(template)
	accessor, err := meta.Accessor(parentObject)
	if err != nil {
		return nil, fmt.Errorf("parentObject does not have ObjectMeta, %v", err)
	}
	prefix := getPodsPrefix(accessor.GetName())

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       desiredLabels,
			Annotations:  desiredAnnotations,
			GenerateName: prefix,
			Finalizers:   desiredFinalizers,
		},
	}
	if controllerRef != nil {
		pod.OwnerReferences = append(pod.OwnerReferences, *controllerRef)
	}
	pod.Spec = *template.Spec.DeepCopy()
	return pod, nil
}

func (r RealPodControl) createPods(nodeName, namespace string, template *v1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	pod, err := GetPodFromTemplate(template, object, controllerRef)
	if err != nil {
		return err
	}
	if len(nodeName) != 0 {
		pod.Spec.NodeName = nodeName
	}
	if labels.Set(pod.Labels).AsSelectorPreValidated().Empty() {
		return fmt.Errorf("unable to create pods, no labels")
	}
	if newPod, err := r.KubeClient.CoreV1().Pods(namespace).Create(pod); err != nil {
		r.Recorder.Eventf(object, v1.EventTypeWarning, FailedCreatePodReason, "Error creating: %v", err)
		return err
	} else {
		accessor, err := meta.Accessor(object)
		if err != nil {
			glog.Errorf("parentObject does not have ObjectMeta, %v", err)
			return nil
		}
		glog.V(4).Infof("Controller %v created pod %v", accessor.GetName(), newPod.Name)
		r.Recorder.Eventf(object, v1.EventTypeNormal, SuccessfulCreatePodReason, "Created pod: %v", newPod.Name)
	}
	return nil
}

func (r RealPodControl) DeletePod(namespace string, podID string, object runtime.Object) error {
	accessor, err := meta.Accessor(object)
	if err != nil {
		return fmt.Errorf("object does not have ObjectMeta, %v", err)
	}
	glog.V(2).Infof("Controller %v deleting pod %v/%v", accessor.GetName(), namespace, podID)
	if err := r.KubeClient.CoreV1().Pods(namespace).Delete(podID, nil); err != nil {
		r.Recorder.Eventf(object, v1.EventTypeWarning, FailedDeletePodReason, "Error deleting: %v", err)
		return fmt.Errorf("unable to delete pods: %v", err)
	} else {
		r.Recorder.Eventf(object, v1.EventTypeNormal, SuccessfulDeletePodReason, "Deleted pod: %v", podID)
	}
	return nil
}

// ServiceControlInterface is an interface that knows how to add or delete Services
// created as an interface to allow testing.
type ServiceControlInterface interface {
	// CreateServices creates new Services according to the spec.
	CreateServices(namespace string, service *v1.Service, object runtime.Object) error
	// CreateServicesWithControllerRef creates new services according to the spec, and sets object as the service's controller.
	CreateServicesWithControllerRef(namespace string, service *v1.Service, object runtime.Object, controllerRef *metav1.OwnerReference) error
	// PatchService patches the service.
	PatchService(namespace, name string, data []byte) error
}

// RealServiceControl is the default implementation of ServiceControlInterface.
type RealServiceControl struct {
	KubeClient clientset.Interface
	Recorder   record.EventRecorder
}

func (r RealServiceControl) PatchService(namespace, name string, data []byte) error {
	_, err := r.KubeClient.CoreV1().Services(namespace).Patch(name, types.StrategicMergePatchType, data)
	return err
}

func (r RealServiceControl) CreateServices(namespace string, service *v1.Service, object runtime.Object) error {
	return r.createServices(namespace, service, object, nil)
}

func (r RealServiceControl) CreateServicesWithControllerRef(namespace string, service *v1.Service, controllerObject runtime.Object, controllerRef *metav1.OwnerReference) error {
	if err := validateControllerRef(controllerRef); err != nil {
		return err
	}
	return r.createServices(namespace, service, controllerObject, controllerRef)
}

func (r RealServiceControl) createServices(namespace string, service *v1.Service, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	if labels.Set(service.Labels).AsSelectorPreValidated().Empty() {
		return fmt.Errorf("unable to create Services, no labels")
	}

	newService, err := r.KubeClient.CoreV1().Services(namespace).Create(service)
	if err != nil {
		r.Recorder.Eventf(object, v1.EventTypeWarning, FailedCreateServiceReason, "Error creating: %v", err)
		return fmt.Errorf("unable to create services: %v", err)
	}

	accessor, err := meta.Accessor(object)
	if err != nil {
		glog.Errorf("parentObject does not have ObjectMeta, %v", err)
		return nil
	}
	glog.V(4).Infof("Controller %v created service %v", accessor.GetName(), newService.Name)
	r.Recorder.Eventf(object, v1.EventTypeNormal, SuccessfulCreateServiceReason, "Created service: %v", newService.Name)

	return nil
}

type FakeServiceControl struct {
	sync.Mutex
	Templates       []v1.Service
	ControllerRefs  []metav1.OwnerReference
	DeletePodName   []string
	Patches         [][]byte
	Err             error
	CreateLimit     int
	CreateCallCount int
}

var _ ServiceControlInterface = &FakeServiceControl{}

func (f *FakeServiceControl) PatchService(namespace, name string, data []byte) error {
	f.Lock()
	defer f.Unlock()
	f.Patches = append(f.Patches, data)
	if f.Err != nil {
		return f.Err
	}
	return nil
}

func (f *FakeServiceControl) CreateServices(namespace string, service *v1.Service, object runtime.Object) error {
	f.Lock()
	defer f.Unlock()
	f.CreateCallCount++
	if f.CreateLimit != 0 && f.CreateCallCount > f.CreateLimit {
		return fmt.Errorf("Not creating service, limit %d already reached (create call %d)", f.CreateLimit, f.CreateCallCount)
	}
	f.Templates = append(f.Templates, *service)
	if f.Err != nil {
		return f.Err
	}
	return nil
}

func (f *FakeServiceControl) CreateServicesWithControllerRef(namespace string, service *v1.Service, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	f.Lock()
	defer f.Unlock()
	f.CreateCallCount++
	if f.CreateLimit != 0 && f.CreateCallCount > f.CreateLimit {
		return fmt.Errorf("Not creating service, limit %d already reached (create call %d)", f.CreateLimit, f.CreateCallCount)
	}
	f.Templates = append(f.Templates, *service)
	f.ControllerRefs = append(f.ControllerRefs, *controllerRef)
	if f.Err != nil {
		return f.Err
	}
	return nil
}
