// Copyright © 2020 VMware
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
	"bytes"
	"encoding/json"

	"github.com/sirupsen/logrus"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// ExtensionsIngressTranslator translates extensions/v1beta1.Ingress objects into
// networking/v1beta1.Ingress objects.
type ExtensionsIngressTranslator struct {

	// Next is the next handler in the chain.
	Next cache.ResourceEventHandler

	Logger logrus.FieldLogger
}

func (e *ExtensionsIngressTranslator) OnAdd(obj interface{}) {
	obj, err := translateExtensionsIngressToNetworkingIngress(obj)
	if err != nil {
		e.Logger.Error(err)
		return
	}
	e.Next.OnAdd(obj)
}

func (e *ExtensionsIngressTranslator) OnUpdate(oldObj, newObj interface{}) {
	oldObj, err := translateExtensionsIngressToNetworkingIngress(oldObj)
	if err != nil {
		e.Logger.Error(err)
		return
	}
	newObj, err = translateExtensionsIngressToNetworkingIngress(newObj)
	if err != nil {
		e.Logger.Error(err)
		return
	}
	e.Next.OnUpdate(oldObj, newObj)
}

func (e *ExtensionsIngressTranslator) OnDelete(obj interface{}) {
	obj, err := translateExtensionsIngressToNetworkingIngress(obj)
	if err != nil {
		e.Logger.Error(err)
		return
	}
	e.Next.OnDelete(obj)
}

// translateExtensionsIngressToNetworkingIngress translates extensions/v1beta1.Ingress
// objects into networking/v1beta1.Ingress objects. If obj is not an extensions/v1beta1.Ingress
// it is returned untouched.
func translateExtensionsIngressToNetworkingIngress(src interface{}) (interface{}, error) {
	if _, ok := src.(*extensionsv1beta1.Ingress); !ok {
		// not an *extensionsv1beta1.Ingress, leave it alone.
		return src, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(src); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(&buf)
	dst := new(v1beta1.Ingress)
	if err := dec.Decode(dst); err != nil {
		return nil, err
	}
	return dst, nil
}
