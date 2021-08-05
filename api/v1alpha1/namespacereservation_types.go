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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceReservationSpec defines the desired state of NamespaceReservation
type NamespaceReservationSpec struct {
	// Duration is how long the reservation will last
	Duration *string `json:"duration,omitempty"`
	// Requester is the entity (bot or human) requesting the namespace
	Requester string `json:"requester"`
}

// NamespaceReservationStatus defines the observed state of NamespaceReservation
type NamespaceReservationStatus struct {
	Expiration string `json:"expiration"`
	Ready      bool   `json:"ready"`
	Namespace  string `json:"namespace"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName=reservation
//+kubebuilder:printcolumn:name="Requester",type="string",JSONPath=".spec.requester"
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
//+kubebuilder:printcolumn:name="Namespace",type="string",JSONPath=".status.namespace"
//+kubebuilder:printcolumn:name="Expiration",type="string",format="date-time",JSONPath=".status.expiration"

// NamespaceReservation is the Schema for the namespacereservations API
type NamespaceReservation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NamespaceReservationSpec   `json:"spec,omitempty"`
	Status NamespaceReservationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NamespaceReservationList contains a list of NamespaceReservation
type NamespaceReservationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NamespaceReservation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NamespaceReservation{}, &NamespaceReservationList{})
}

// MakeOwnerReference defines the owner reference pointing to the NamespaceReservation resource.
func (i *NamespaceReservation) MakeOwnerReference() metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: i.APIVersion,
		Kind:       i.Kind,
		Name:       i.ObjectMeta.Name,
		UID:        i.ObjectMeta.UID,
		Controller: TruePtr(),
	}
}

// TruePtr returns a pointer to True
func TruePtr() *bool {
	t := true
	return &t
}
