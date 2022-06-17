// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package v1alpha1

import (
	vzapi "github.com/verrazzano/verrazzano/platform-operator/apis/verrazzano/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ModuleSpec struct {
	Installer    ModuleInstaller    `json:"installer"`
	Dependencies []ModuleDependency `json:"dependencies"`
}

type ModuleInstaller struct {
	HelmChart *HelmChart `json:"helmChart,omitempty"`
}

type HelmChart struct {
	Name                   string         `json:"name"`
	Namespace              string         `json:"namespace,omitempty"`
	Repository             HelmRepository `json:"repository,omitempty"`
	Version                string         `json:"version,omitempty"`
	vzapi.InstallOverrides `json:",inline"`
}

type HelmRepository struct {
	Path      string `json:"path,omitempty"`
	URI       string `json:"uri,omitempty"`
	SecretRef string `json:"secretRef,omitempty"`
}

type ModuleDependency struct {
}

// ModuleStatus defines the observed state of Module
type ModuleStatus struct {
	Phase              *ModulePhase `json:"phase,omitempty"`
	ObservedGeneration int64        `json:"observedGeneration,omitempty"`
	ReconciledAt       *string      `json:"reconciledAt,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="Phase of Module reconciliation"
// +genclient

// Module is the Schema for the modules API
type Module struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModuleSpec   `json:"spec,omitempty"`
	Status ModuleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ModuleList contains a list of Module
type ModuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Module `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Module{}, &ModuleList{})
}

func (m *Module) ChartNamespace() string {
	if m.Spec.Installer.HelmChart == nil {
		return m.Namespace
	}
	return m.Spec.Installer.HelmChart.Namespace
}

func (m *Module) IsBeingDeleted() bool {
	return m != nil && m.GetDeletionTimestamp() != nil
}
