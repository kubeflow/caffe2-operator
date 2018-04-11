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

package fake

import (
	v1alpha1 "github.com/kubeflow/caffe2-operator/pkg/apis/caffe2/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeCaffe2Jobs implements Caffe2JobInterface
type FakeCaffe2Jobs struct {
	Fake *FakeKubeflowV1alpha1
	ns   string
}

var caffe2jobsResource = schema.GroupVersionResource{Group: "kubeflow.org", Version: "v1alpha1", Resource: "caffe2jobs"}

var caffe2jobsKind = schema.GroupVersionKind{Group: "kubeflow.org", Version: "v1alpha1", Kind: "Caffe2Job"}

// Get takes name of the caffe2Job, and returns the corresponding caffe2Job object, and an error if there is any.
func (c *FakeCaffe2Jobs) Get(name string, options v1.GetOptions) (result *v1alpha1.Caffe2Job, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(caffe2jobsResource, c.ns, name), &v1alpha1.Caffe2Job{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Caffe2Job), err
}

// List takes label and field selectors, and returns the list of Caffe2Jobs that match those selectors.
func (c *FakeCaffe2Jobs) List(opts v1.ListOptions) (result *v1alpha1.Caffe2JobList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(caffe2jobsResource, caffe2jobsKind, c.ns, opts), &v1alpha1.Caffe2JobList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.Caffe2JobList{}
	for _, item := range obj.(*v1alpha1.Caffe2JobList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested caffe2Jobs.
func (c *FakeCaffe2Jobs) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(caffe2jobsResource, c.ns, opts))

}

// Create takes the representation of a caffe2Job and creates it.  Returns the server's representation of the caffe2Job, and an error, if there is any.
func (c *FakeCaffe2Jobs) Create(caffe2Job *v1alpha1.Caffe2Job) (result *v1alpha1.Caffe2Job, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(caffe2jobsResource, c.ns, caffe2Job), &v1alpha1.Caffe2Job{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Caffe2Job), err
}

// Update takes the representation of a caffe2Job and updates it. Returns the server's representation of the caffe2Job, and an error, if there is any.
func (c *FakeCaffe2Jobs) Update(caffe2Job *v1alpha1.Caffe2Job) (result *v1alpha1.Caffe2Job, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(caffe2jobsResource, c.ns, caffe2Job), &v1alpha1.Caffe2Job{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Caffe2Job), err
}

// Delete takes name of the caffe2Job and deletes it. Returns an error if one occurs.
func (c *FakeCaffe2Jobs) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(caffe2jobsResource, c.ns, name), &v1alpha1.Caffe2Job{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeCaffe2Jobs) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(caffe2jobsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.Caffe2JobList{})
	return err
}

// Patch applies the patch and returns the patched caffe2Job.
func (c *FakeCaffe2Jobs) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Caffe2Job, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(caffe2jobsResource, c.ns, name, data, subresources...), &v1alpha1.Caffe2Job{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Caffe2Job), err
}
