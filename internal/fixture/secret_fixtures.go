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
	core_v1 "k8s.io/api/core/v1"
)

var SecretRootsCert = &core_v1.Secret{
	ObjectMeta: ObjectMeta("roots/ssl-cert"),
	Type:       core_v1.SecretTypeTLS,
	Data: map[string][]byte{
		core_v1.TLSCertKey:       []byte(CERTIFICATE),
		core_v1.TLSPrivateKeyKey: []byte(RSA_PRIVATE_KEY),
	},
}

var SecretProjectContourCert = &core_v1.Secret{
	ObjectMeta: ObjectMeta("projectcontour/default-ssl-cert"),
	Type:       core_v1.SecretTypeTLS,
	Data:       SecretRootsCert.Data,
}

var SecretRootsFallback = &core_v1.Secret{
	ObjectMeta: ObjectMeta("roots/fallbacksecret"),
	Type:       core_v1.SecretTypeTLS,
	Data: map[string][]byte{
		core_v1.TLSCertKey:       []byte(CERTIFICATE),
		core_v1.TLSPrivateKeyKey: []byte(RSA_PRIVATE_KEY),
	},
}
