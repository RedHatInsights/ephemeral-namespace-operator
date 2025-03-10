//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespacePool) DeepCopyInto(out *NamespacePool) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespacePool.
func (in *NamespacePool) DeepCopy() *NamespacePool {
	if in == nil {
		return nil
	}
	out := new(NamespacePool)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NamespacePool) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespacePoolList) DeepCopyInto(out *NamespacePoolList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]NamespacePool, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespacePoolList.
func (in *NamespacePoolList) DeepCopy() *NamespacePoolList {
	if in == nil {
		return nil
	}
	out := new(NamespacePoolList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NamespacePoolList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespacePoolSpec) DeepCopyInto(out *NamespacePoolSpec) {
	*out = *in
	if in.SizeLimit != nil {
		in, out := &in.SizeLimit, &out.SizeLimit
		*out = new(int)
		**out = **in
	}
	in.ClowdEnvironment.DeepCopyInto(&out.ClowdEnvironment)
	in.LimitRange.DeepCopyInto(&out.LimitRange)
	in.ResourceQuotas.DeepCopyInto(&out.ResourceQuotas)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespacePoolSpec.
func (in *NamespacePoolSpec) DeepCopy() *NamespacePoolSpec {
	if in == nil {
		return nil
	}
	out := new(NamespacePoolSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespacePoolStatus) DeepCopyInto(out *NamespacePoolStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespacePoolStatus.
func (in *NamespacePoolStatus) DeepCopy() *NamespacePoolStatus {
	if in == nil {
		return nil
	}
	out := new(NamespacePoolStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespaceReservation) DeepCopyInto(out *NamespaceReservation) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespaceReservation.
func (in *NamespaceReservation) DeepCopy() *NamespaceReservation {
	if in == nil {
		return nil
	}
	out := new(NamespaceReservation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NamespaceReservation) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespaceReservationList) DeepCopyInto(out *NamespaceReservationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]NamespaceReservation, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespaceReservationList.
func (in *NamespaceReservationList) DeepCopy() *NamespaceReservationList {
	if in == nil {
		return nil
	}
	out := new(NamespaceReservationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NamespaceReservationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespaceReservationSpec) DeepCopyInto(out *NamespaceReservationSpec) {
	*out = *in
	if in.Duration != nil {
		in, out := &in.Duration, &out.Duration
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespaceReservationSpec.
func (in *NamespaceReservationSpec) DeepCopy() *NamespaceReservationSpec {
	if in == nil {
		return nil
	}
	out := new(NamespaceReservationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespaceReservationStatus) DeepCopyInto(out *NamespaceReservationStatus) {
	*out = *in
	in.Expiration.DeepCopyInto(&out.Expiration)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespaceReservationStatus.
func (in *NamespaceReservationStatus) DeepCopy() *NamespaceReservationStatus {
	if in == nil {
		return nil
	}
	out := new(NamespaceReservationStatus)
	in.DeepCopyInto(out)
	return out
}
