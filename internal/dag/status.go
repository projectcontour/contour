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

func (sw *StatusWriter) WithObject(obj Object) *ObjectStatusWriter {
	return &ObjectStatusWriter{
		sw:     sw,
		obj:    obj,
		values: make(map[string]string),
	}
}

func (osw *ObjectStatusWriter) WithValue(key, val string) *ObjectStatusWriter {
	osw.values[key] = val
	return osw
}

func (osw *ObjectStatusWriter) SetInvalid(desc string) *ObjectStatusWriter {
	return osw.WithValue("description", desc).WithValue("status", StatusInvalid)
}

func (osw *ObjectStatusWriter) SetValid() *ObjectStatusWriter {
	return osw.WithValue("status", StatusValid)
}

func (osw *ObjectStatusWriter) WithObject(obj Object) *ObjectStatusWriter {
	m := make(map[string]string)
	for k, v := range osw.values {
		m[k] = v
	}
	return &ObjectStatusWriter{
		sw:     osw.sw,
		obj:    obj,
		values: m,
	}
}

func (osw *ObjectStatusWriter) Commit() {
	if len(osw.values) == 0 {
		// nothing to commit
		return
	}

	m := Meta{
		name:      osw.obj.GetObjectMeta().GetName(),
		namespace: osw.obj.GetObjectMeta().GetNamespace(),
	}
	if _, ok := osw.sw.statuses[m]; !ok {
		// only record the first status event
		osw.sw.statuses[m] = Status{
			Object:      osw.obj,
			Status:      osw.values["status"],
			Description: osw.values["description"],
			Vhost:       osw.values["vhost"],
		}
	}
}
