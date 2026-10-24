package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto copies all properties of this object into another object of the same type.
func (in *EnvVar) DeepCopyInto(out *EnvVar) {
	*out = *in
}

func (in *EnvVar) DeepCopy() *EnvVar {
	if in == nil {
		return nil
	}
	out := new(EnvVar)
	in.DeepCopyInto(out)
	return out
}

func (in *ResourceSpec) DeepCopyInto(out *ResourceSpec) {
	*out = *in
}

func (in *ResourceSpec) DeepCopy() *ResourceSpec {
	if in == nil {
		return nil
	}
	out := new(ResourceSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *PreviewEnvironmentSpec) DeepCopyInto(out *PreviewEnvironmentSpec) {
	*out = *in
	out.Resources = in.Resources
	if in.Env != nil {
		in, out := &in.Env, &out.Env
		*out = make([]EnvVar, len(*in))
		copy(*out, *in)
	}
}

func (in *PreviewEnvironmentSpec) DeepCopy() *PreviewEnvironmentSpec {
	if in == nil {
		return nil
	}
	out := new(PreviewEnvironmentSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *PreviewEnvironmentStatus) DeepCopyInto(out *PreviewEnvironmentStatus) {
	*out = *in
	if in.CreatedAt != nil {
		in, out := &in.CreatedAt, &out.CreatedAt
		*out = (*in).DeepCopy()
	}
	if in.ReadyAt != nil {
		in, out := &in.ReadyAt, &out.ReadyAt
		*out = (*in).DeepCopy()
	}
	if in.ExpiresAt != nil {
		in, out := &in.ExpiresAt, &out.ExpiresAt
		*out = (*in).DeepCopy()
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *PreviewEnvironmentStatus) DeepCopy() *PreviewEnvironmentStatus {
	if in == nil {
		return nil
	}
	out := new(PreviewEnvironmentStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *PreviewEnvironment) DeepCopyInto(out *PreviewEnvironment) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *PreviewEnvironment) DeepCopy() *PreviewEnvironment {
	if in == nil {
		return nil
	}
	out := new(PreviewEnvironment)
	in.DeepCopyInto(out)
	return out
}

func (in *PreviewEnvironment) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *PreviewEnvironmentList) DeepCopyInto(out *PreviewEnvironmentList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PreviewEnvironment, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *PreviewEnvironmentList) DeepCopy() *PreviewEnvironmentList {
	if in == nil {
		return nil
	}
	out := new(PreviewEnvironmentList)
	in.DeepCopyInto(out)
	return out
}

func (in *PreviewEnvironmentList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
