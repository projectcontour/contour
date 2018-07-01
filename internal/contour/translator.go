// Copyright © 2017 Heptio
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

// Package contour contains the translation business logic that listens
// to Kubernetes ResourceEventHandler events and translates those into
// additions/deletions in caches connected to the Envoy xDS gRPC API server.
package contour

import (
	"strings"

	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const DEFAULT_INGRESS_CLASS = "contour"

// Translator receives notifications from the Kubernetes API and translates those
// objects into additions and removals entries of Envoy gRPC objects from a cache.
type Translator struct {
	// The logger for this Translator. There is no valid default, this value
	// must be supplied by the caller.
	logrus.FieldLogger

	// Contour's IngressClass.
	// If not set, defaults to DEFAULT_INGRESS_CLASS.
	IngressClass string
}

func (t *Translator) OnAdd(obj interface{}) {
	t.Errorf("OnAdd unexpected type %T: %#v", obj, obj)
}

func (t *Translator) OnUpdate(oldObj, newObj interface{}) {
	t.Errorf("OnUpdate unexpected type %T: %#v", newObj, newObj)
}

func (t *Translator) OnDelete(obj interface{}) {
	t.Errorf("OnDelete unexpected type %T: %#v", obj, obj)
}

// servicename returns a fixed name for this service and portname
func servicename(meta metav1.ObjectMeta, portname string) string {
	sn := []string{
		meta.Namespace,
		meta.Name,
		"",
	}[:2]
	if portname != "" {
		sn = append(sn, portname)
	}
	return strings.Join(sn, "/")
}
