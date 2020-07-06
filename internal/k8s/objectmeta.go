// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/strings"
)

// Object is any Kubernetes object that has an ObjectMeta.
// TODO(youngnick): Review references to this and replace them
// with straight metav1.ObjectMetaAccessor calls if we can.
type Object interface {
	metav1.ObjectMetaAccessor
}

// FullName holds the name and namespace of a Kubernetes object.
type FullName struct {
	Name, Namespace string
}

// String returns a string representation of the name.
func (f FullName) String() string {
	if f.Name == "" {
		return ""
	}

	ns := f.Namespace
	if ns == "" {
		ns = metav1.NamespaceDefault
	}

	return strings.JoinQualifiedName(ns, f.Name)
}

// ToFullName returns the FullName of any given Kubernetes object.
func ToFullName(obj Object) FullName {
	m := obj.GetObjectMeta()
	return FullName{
		Name:      m.GetName(),
		Namespace: m.GetNamespace(),
	}
}
