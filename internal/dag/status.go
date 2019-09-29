// Copyright © 2019 VMware
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

package dag

import (
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	StatusValid    = "valid"
	StatusInvalid  = "invalid"
	StatusOrphaned = "orphaned"
)

// Status contains the status for an IngressRoute (valid / invalid / orphan, etc)
type Status struct {
	Object      Object
	Status      string
	Description string
	Vhost       string
}

type StatusWriter struct {
	statuses map[Meta]Status
}

type Object interface {
	metav1.ObjectMetaAccessor
}

type ObjectStatusWriter struct {
	sw     *StatusWriter
	obj    Object
	values map[string]string
}

// WithObject returns an ObjectStatusWriter that can be used to set the state of
// the Object. The state can be set as many times as necessary. The state of the
// object can be made perminent by calling the commit function returned from WithObject.
// The caller should pass the ObjectStatusWriter to functions interested in writing status,
// but keep the commit function for itself. The commit function should be either called
// via a defer, or directly if statuses are being set in a loop (as defers will not fire
// until the end of the function).
func (sw *StatusWriter) WithObject(obj Object) (_ *ObjectStatusWriter, commit func()) {
	osw := &ObjectStatusWriter{
		sw:     sw,
		obj:    obj,
		values: make(map[string]string),
	}
	return osw, func() {
		sw.commit(osw)
	}
}

func (sw *StatusWriter) commit(osw *ObjectStatusWriter) {
	if len(osw.values) == 0 {
		// nothing to commit
		return
	}

	m := Meta{
		name:      osw.obj.GetObjectMeta().GetName(),
		namespace: osw.obj.GetObjectMeta().GetNamespace(),
	}
	if _, ok := sw.statuses[m]; !ok {
		// only record the first status event
		sw.statuses[m] = Status{
			Object:      osw.obj,
			Status:      osw.values["status"],
			Description: osw.values["description"],
			Vhost:       osw.values["vhost"],
		}
	}
}
func (osw *ObjectStatusWriter) WithValue(key, val string) *ObjectStatusWriter {
	osw.values[key] = val
	return osw
}

func (osw *ObjectStatusWriter) SetInvalid(desc string) {
	osw.WithValue("description", desc).WithValue("status", StatusInvalid)
}

func (osw *ObjectStatusWriter) SetValid() {
	switch osw.obj.(type) {
	case *projcontour.HTTPProxy:
		osw.WithValue("description", "valid HTTPProxy").WithValue("status", StatusValid)
	case *ingressroutev1.IngressRoute:
		osw.WithValue("description", "valid IngressRoute").WithValue("status", StatusValid)
	default:
		// not a supported type
	}
}

// WithObject returns a new ObjectStatusWriter with a copy of the current
// ObjectStatusWriter's values, including its status if set. This is convenient if
// the object shares a relationship with its parent. The caller should arrange for
// the commit function to be called to write the final status of the object.
func (osw *ObjectStatusWriter) WithObject(obj Object) (_ *ObjectStatusWriter, commit func()) {
	m := make(map[string]string)
	for k, v := range osw.values {
		m[k] = v
	}
	nosw := &ObjectStatusWriter{
		sw:     osw.sw,
		obj:    obj,
		values: m,
	}
	return nosw, func() {
		osw.sw.commit(nosw)
	}
}
