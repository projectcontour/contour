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

package dag

import (
	"fmt"

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
)

// Status contains the status for an HTTPProxy (valid / invalid / orphan, etc)
type Status struct {
	Object      k8s.Object
	Status      string
	Description string
	Vhost       string
}

type StatusWriter struct {
	statuses map[k8s.FullName]Status
}

type ObjectStatusWriter struct {
	sw     *StatusWriter
	obj    k8s.Object
	values map[string]string
}

// WithObject returns an ObjectStatusWriter that can be used to set the state of
// the object. The state can be set as many times as necessary. The state of the
// object can be made permanent by calling the commit function returned from WithObject.
// The caller should pass the ObjectStatusWriter to functions interested in writing status,
// but keep the commit function for itself. The commit function should be either called
// via a defer, or directly if statuses are being set in a loop (as defers will not fire
// until the end of the function).
func (sw *StatusWriter) WithObject(obj k8s.Object) (_ *ObjectStatusWriter, commit func()) {
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

	m := k8s.FullName{
		Name:      osw.obj.GetObjectMeta().GetName(),
		Namespace: osw.obj.GetObjectMeta().GetNamespace(),
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

func (osw *ObjectStatusWriter) SetInvalid(format string, args ...interface{}) {
	osw.WithValue("description", fmt.Sprintf(format, args...)).WithValue("status", k8s.StatusInvalid)
}

func (osw *ObjectStatusWriter) SetValid() {
	switch osw.obj.(type) {
	case *projcontour.HTTPProxy:
		osw.WithValue("description", "valid HTTPProxy").WithValue("status", k8s.StatusValid)
	default:
		// not a supported type
	}
}

// WithObject returns a new ObjectStatusWriter with a copy of the current
// ObjectStatusWriter's values, including its status if set. This is convenient if
// the object shares a relationship with its parent. The caller should arrange for
// the commit function to be called to write the final status of the object.
func (osw *ObjectStatusWriter) WithObject(obj k8s.Object) (_ *ObjectStatusWriter, commit func()) {
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
