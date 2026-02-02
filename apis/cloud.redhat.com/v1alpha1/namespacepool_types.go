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
	// Number of namespaces to have ready in the pool
	Size int `json:"size"`
	// Optional max number of namespaces for a pool
	SizeLimit *int `json:"sizeLimit,omitempty"`
	// Determine whether the project uses a project or a namespace
	Local bool `json:"local"`
	// Clowdenvironment template for the namespace
	ClowdEnvironment clowder.ClowdEnvironmentSpec `json:"clowdenvironment"`
	// Resource limits for containers and pods for the deployed namespace
	LimitRange core.LimitRange `json:"limitrange"`
	// Defined resource quotas for specific states for the deployed namespace
	ResourceQuotas core.ResourceQuotaList `json:"resourcequotas"`
	// Description for the namespace pool
	Description string `json:"description,omitempty"`
	// Contains a list of teams and corresponding secrets
	Teams []Team `json:"teams,omitempty"`
}

// Team defines options that alter ENO behavior depending on name of team making the reservation
type Team struct {
	Name    string        `json:"name"`
	Secrets []SecretsData `json:"secrets"`
}

// SecretsData defines secret name mapping for team-specific secrets
type SecretsData struct {
	Name     string `json:"name"`
	DestName string `json:"destName,omitempty"`
}

// NamespacePoolStatus defines the observed state of Pool
type NamespacePoolStatus struct {
	Ready    int `json:"ready"`
	Creating int `json:"creating"`
	Reserved int `json:"reserved"`
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

// NamespacePoolList contains a list of NamespacePool
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
		Name:       i.Name,
		UID:        i.UID,
		Controller: TruePtr(),
	}
}
