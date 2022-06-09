/*
Copyright 2021.

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
	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespacePoolSpec defines the desired state of Pool
type NamespacePoolSpec struct {
	Size             int                          `json:"size"`
	Local            bool                         `json:"local"`
	ClowdEnvironment clowder.ClowdEnvironmentSpec `json:"clowdenvironment"`
	LimitRange       core.LimitRange              `json:"limitrange"`
	ResourceQuotas   core.ResourceQuotaList       `json:"resourcequotas"`
	Description      string                       `json:"description,omitempty"`
}

// NamespacePoolStatus defines the observed state of Pool
type NamespacePoolStatus struct {
	Ready    int `json:"ready"`
	Creating int `json:"creating"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName=nspool
//+kubebuilder:printcolumn:name="Pool Size",type="string",JSONPath=".spec.size"
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready"
//+kubebuilder:printcolumn:name="Creating",type="string",JSONPath=".status.creating"

// NamespacePool is the Schema for the pools API
type NamespacePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NamespacePoolSpec   `json:"spec,omitempty"`
	Status NamespacePoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PoolList contains a list of Pool
type NamespacePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NamespacePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NamespacePool{}, &NamespacePoolList{})
}

// MakeOwnerReference defines the owner reference pointing to the Pool resource.
func (i *NamespacePool) MakeOwnerReference() metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: i.APIVersion,
		Kind:       i.Kind,
		Name:       i.ObjectMeta.Name,
		UID:        i.ObjectMeta.UID,
		Controller: TruePtr(),
	}
}
