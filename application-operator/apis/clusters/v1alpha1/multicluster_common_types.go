// Copyright (c) 2021, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package v1alpha1

import corev1 "k8s.io/api/core/v1"

// This file contains common types and functions used by all MultiCluster Custom Resource Types

// Placement information for multi cluster resources
type Placement struct {
	Clusters []Cluster `json:"clusters"`
}

// Cluster where multi cluster resources are placed
type Cluster struct {
	// the name of the cluster
	Name string `json:"name"`
}

// Condition describes current state of a multi cluster resource.
type Condition struct {
	// Type of condition.
	Type ConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// ClusterLevelStatus describes the status of the multi cluster resource in a specific cluster
type ClusterLevelStatus struct {
	// Name of the cluster
	Name string `json:"name"`
	// State of the resource in this cluster
	State StateType `json:"state"`
	// Message with details about the status in this cluster
	Message string `json:"message,omitempty"`
	// LastUpdateTime of the resource state in this cluster
	LastUpdateTime string `json:"lastUpdateTime"`
}

// ConditionType identifies the condition of the multi-cluster resource which can be checked with kubectl wait
type ConditionType string

const (
	// DeployPending means deployment to specified cluster is in progress.
	DeployPending ConditionType = "DeployPending"

	// DeployComplete means deployment to specified cluster completed successfully
	DeployComplete ConditionType = "DeployComplete"

	// DeployFailed means the deployment to specified cluster has failed.
	DeployFailed ConditionType = "DeployFailed"
)

// StateType identifies the state of a multi-cluster resource
type StateType string

const (
	// Pending is the state when deploy to specified cluster is in progress
	Pending StateType = "Pending"

	// Succeeded is the state when deploy to specified cluster is completed
	Succeeded StateType = "Succeeded"

	// Failed is the state when deploy to specified cluster has failed
	Failed StateType = "Failed"
)

// MultiClusterResourceStatus represents the status of a multi-cluster resource, including
// cluster-level status information
type MultiClusterResourceStatus struct {
	// The latest available observations of an object's current state.
	Conditions []Condition `json:"conditions,omitempty"`

	// State of the multi cluster resource
	State StateType `json:"state,omitempty"`

	Clusters []ClusterLevelStatus `json:"clusters,omitempty"`
}

type EmbeddedObjectMeta struct {
	// Name must be unique within a namespace.
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`

	// Namespace defines the space within each name must be unique.
	// +optional
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,3,opt,name=namespace"`

	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects.
	// +optional
	Labels map[string]string `json:"labels,omitempty" protobuf:"bytes,11,rep,name=labels"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,12,rep,name=annotations"`
}
