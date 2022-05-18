// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DNSSpec defines the desired state of DNS
type DNSSpec struct {
	// DNS type of wildcard.  This is the default.
	// +optional
	Wildcard *Wildcard `json:"wildcard,omitempty"`
	// DNS type of OCI (Oracle Cloud Infrastructure)
	// +optional
	OCI *OCI `json:"oci,omitempty"`
	// DNS type of external. For example, OLCNE uses this type.
	// +optional
	External *External `json:"external,omitempty"`
}

// DNSStatus defines the observed state of DNS
type DNSStatus struct {
	// The latest available observations of an object's current state.
	Conditions []Condition `json:"conditions,omitempty"`
	// State of the DNS custom resource
	State string `json:"state,omitempty"`
}

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

// ConditionType identifies the condition of the install/uninstall/upgrade which can be checked with kubectl wait
type ConditionType string

// Wildcard DNS type
type Wildcard struct {
	// DNS wildcard domain (nip.io, sslip.io, etc.)
	Domain string `json:"domain"`
}

// OCI DNS type
type OCI struct {
	OCIConfigSecret        string `json:"ociConfigSecret"`
	DNSZoneCompartmentOCID string `json:"dnsZoneCompartmentOCID"`
	DNSZoneOCID            string `json:"dnsZoneOCID"`
	DNSZoneName            string `json:"dnsZoneName"`
	DNSScope               string `json:"dnsScope,omitempty"`
}

// External DNS type
type External struct {
	// DNS suffix appended to EnviromentName to form DNS name
	Suffix string `json:"suffix"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:status
// +genclient
type DNS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DNSSpec   `json:"spec,omitempty"`
	Status DNSStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DNSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNS `json:"items"`
}
