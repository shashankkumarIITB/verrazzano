// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/verrazzano/verrazzano/platform-operator/apis/clusters/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeVerrazzanoManagedClusters implements VerrazzanoManagedClusterInterface
type FakeVerrazzanoManagedClusters struct {
	Fake *FakeClustersV1alpha1
	ns   string
}

var verrazzanomanagedclustersResource = schema.GroupVersionResource{Group: "clusters.verrazzano.io", Version: "v1alpha1", Resource: "verrazzanomanagedclusters"}

var verrazzanomanagedclustersKind = schema.GroupVersionKind{Group: "clusters.verrazzano.io", Version: "v1alpha1", Kind: "VerrazzanoManagedCluster"}

// Get takes name of the verrazzanoManagedCluster, and returns the corresponding verrazzanoManagedCluster object, and an error if there is any.
func (c *FakeVerrazzanoManagedClusters) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.VerrazzanoManagedCluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(verrazzanomanagedclustersResource, c.ns, name), &v1alpha1.VerrazzanoManagedCluster{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VerrazzanoManagedCluster), err
}

// List takes label and field selectors, and returns the list of VerrazzanoManagedClusters that match those selectors.
func (c *FakeVerrazzanoManagedClusters) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.VerrazzanoManagedClusterList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(verrazzanomanagedclustersResource, verrazzanomanagedclustersKind, c.ns, opts), &v1alpha1.VerrazzanoManagedClusterList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.VerrazzanoManagedClusterList{ListMeta: obj.(*v1alpha1.VerrazzanoManagedClusterList).ListMeta}
	for _, item := range obj.(*v1alpha1.VerrazzanoManagedClusterList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested verrazzanoManagedClusters.
func (c *FakeVerrazzanoManagedClusters) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(verrazzanomanagedclustersResource, c.ns, opts))

}

// Create takes the representation of a verrazzanoManagedCluster and creates it.  Returns the server's representation of the verrazzanoManagedCluster, and an error, if there is any.
func (c *FakeVerrazzanoManagedClusters) Create(ctx context.Context, verrazzanoManagedCluster *v1alpha1.VerrazzanoManagedCluster, opts v1.CreateOptions) (result *v1alpha1.VerrazzanoManagedCluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(verrazzanomanagedclustersResource, c.ns, verrazzanoManagedCluster), &v1alpha1.VerrazzanoManagedCluster{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VerrazzanoManagedCluster), err
}

// Update takes the representation of a verrazzanoManagedCluster and updates it. Returns the server's representation of the verrazzanoManagedCluster, and an error, if there is any.
func (c *FakeVerrazzanoManagedClusters) Update(ctx context.Context, verrazzanoManagedCluster *v1alpha1.VerrazzanoManagedCluster, opts v1.UpdateOptions) (result *v1alpha1.VerrazzanoManagedCluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(verrazzanomanagedclustersResource, c.ns, verrazzanoManagedCluster), &v1alpha1.VerrazzanoManagedCluster{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VerrazzanoManagedCluster), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeVerrazzanoManagedClusters) UpdateStatus(ctx context.Context, verrazzanoManagedCluster *v1alpha1.VerrazzanoManagedCluster, opts v1.UpdateOptions) (*v1alpha1.VerrazzanoManagedCluster, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(verrazzanomanagedclustersResource, "status", c.ns, verrazzanoManagedCluster), &v1alpha1.VerrazzanoManagedCluster{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VerrazzanoManagedCluster), err
}

// Delete takes name of the verrazzanoManagedCluster and deletes it. Returns an error if one occurs.
func (c *FakeVerrazzanoManagedClusters) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(verrazzanomanagedclustersResource, c.ns, name, opts), &v1alpha1.VerrazzanoManagedCluster{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVerrazzanoManagedClusters) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(verrazzanomanagedclustersResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.VerrazzanoManagedClusterList{})
	return err
}

// Patch applies the patch and returns the patched verrazzanoManagedCluster.
func (c *FakeVerrazzanoManagedClusters) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.VerrazzanoManagedCluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(verrazzanomanagedclustersResource, c.ns, name, pt, data, subresources...), &v1alpha1.VerrazzanoManagedCluster{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VerrazzanoManagedCluster), err
}
