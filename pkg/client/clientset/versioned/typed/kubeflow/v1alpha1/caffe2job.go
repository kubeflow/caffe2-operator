/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	v1alpha1 "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
	scheme "github.com/kubeflow/caffe2-operator/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// Caffe2JobsGetter has a method to return a Caffe2JobInterface.
// A group's client should implement this interface.
type Caffe2JobsGetter interface {
	Caffe2Jobs(namespace string) Caffe2JobInterface
}

// Caffe2JobInterface has methods to work with Caffe2Job resources.
type Caffe2JobInterface interface {
	Create(*v1alpha1.Caffe2Job) (*v1alpha1.Caffe2Job, error)
	Update(*v1alpha1.Caffe2Job) (*v1alpha1.Caffe2Job, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.Caffe2Job, error)
	List(opts v1.ListOptions) (*v1alpha1.Caffe2JobList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Caffe2Job, err error)
	Caffe2JobExpansion
}

// caffe2Jobs implements Caffe2JobInterface
type caffe2Jobs struct {
	client rest.Interface
	ns     string
}

// newCaffe2Jobs returns a Caffe2Jobs
func newCaffe2Jobs(c *KubeflowV1alpha1Client, namespace string) *caffe2Jobs {
	return &caffe2Jobs{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the caffe2Job, and returns the corresponding caffe2Job object, and an error if there is any.
func (c *caffe2Jobs) Get(name string, options v1.GetOptions) (result *v1alpha1.Caffe2Job, err error) {
	result = &v1alpha1.Caffe2Job{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("caffe2jobs").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Caffe2Jobs that match those selectors.
func (c *caffe2Jobs) List(opts v1.ListOptions) (result *v1alpha1.Caffe2JobList, err error) {
	result = &v1alpha1.Caffe2JobList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("caffe2jobs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested caffe2Jobs.
func (c *caffe2Jobs) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("caffe2jobs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a caffe2Job and creates it.  Returns the server's representation of the caffe2Job, and an error, if there is any.
func (c *caffe2Jobs) Create(caffe2Job *v1alpha1.Caffe2Job) (result *v1alpha1.Caffe2Job, err error) {
	result = &v1alpha1.Caffe2Job{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("caffe2jobs").
		Body(caffe2Job).
		Do().
		Into(result)
	return
}

// Update takes the representation of a caffe2Job and updates it. Returns the server's representation of the caffe2Job, and an error, if there is any.
func (c *caffe2Jobs) Update(caffe2Job *v1alpha1.Caffe2Job) (result *v1alpha1.Caffe2Job, err error) {
	result = &v1alpha1.Caffe2Job{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("caffe2jobs").
		Name(caffe2Job.Name).
		Body(caffe2Job).
		Do().
		Into(result)
	return
}

// Delete takes name of the caffe2Job and deletes it. Returns an error if one occurs.
func (c *caffe2Jobs) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("caffe2jobs").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *caffe2Jobs) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("caffe2jobs").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched caffe2Job.
func (c *caffe2Jobs) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Caffe2Job, err error) {
	result = &v1alpha1.Caffe2Job{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("caffe2jobs").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
