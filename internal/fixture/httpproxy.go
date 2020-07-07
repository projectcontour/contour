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

package fixture

import (
	v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// ProxyBuilder is a builder object to make creating HTTPProxy fixtures more succinct.
type ProxyBuilder v1.HTTPProxy

// NewProxy creates a new ProxyBuilder with the specified object name.
func NewProxy(name string) *ProxyBuilder {
	b := &ProxyBuilder{
		ObjectMeta: ObjectMeta(name),
	}

	b.ObjectMeta.Annotations = map[string]string{}
	b.ObjectMeta.Labels = map[string]string{}

	return b
}

// Label adds the given values as metadata annotations.
func (b *ProxyBuilder) Annotate(k string, v string) *ProxyBuilder {
	b.ObjectMeta.Annotations[k] = v
	return b
}

// Label adds the given values as metadata labels.
func (b *ProxyBuilder) Label(k string, v string) *ProxyBuilder {
	b.ObjectMeta.Labels[k] = v
	return b
}

// WithSpec updates the builder's Spec field, returning the build proxy.
func (b *ProxyBuilder) WithSpec(spec v1.HTTPProxySpec) *v1.HTTPProxy {
	oldSpec := b.Spec

	b.Spec = spec

	// TODO(jpeach): use a full merge library so that updating
	// fields then finishing with a spec is ordering insensitive.
	if b.Spec.VirtualHost == nil {
		b.Spec.VirtualHost = oldSpec.VirtualHost
	}

	return (*v1.HTTPProxy)(b)
}

func (b *ProxyBuilder) WithFQDN(fqdn string) *ProxyBuilder {
	if b.Spec.VirtualHost == nil {
		b.Spec.VirtualHost = &v1.VirtualHost{}
	}

	b.Spec.VirtualHost.Fqdn = fqdn
	return b
}
