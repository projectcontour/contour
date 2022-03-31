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

package certgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAsSecrets_AsLegacySecrets_InvalidNamespaceAndName(t *testing.T) {
	tests := map[string]struct {
		namespace  string
		nameSuffix string
		wantErrs   int
	}{
		"invalid namespace characters": {
			namespace:  "invalid-namespace-chars!",
			nameSuffix: "-valid-suffix",
			wantErrs:   1,
		},
		"namespace name too long": {
			namespace:  "this-namespace-name-is-way-too-long-and-surely-wont-be-allowed-by-the-api-server",
			nameSuffix: "-valid-suffix",
			wantErrs:   1,
		},
		"invalid name suffix characters": {
			namespace:  "valid-namespace",
			nameSuffix: "-invalid-namesuffix-chars$",
			wantErrs:   1,
		},
		"name suffix too long": {
			namespace:  "valid-namespace",
			nameSuffix: "this-name-suffix-is-way-too-long-and-surely-wont-be-allowed-by-the-api-server-but-wow-secret-names-are-allowed-to-be-really-long-so-this-test-data-has-to-be-extremely-lengthy-in-order-to-trigger-an-error-running-out-of-words-now-but-this-will-just-keep-going-until-i-see-a-green-check",
			wantErrs:   1,
		},
		"invalid namespace and name suffix characters": {
			namespace:  "invalid-namespace-chars!",
			nameSuffix: "-invalid-namesuffix-chars$",
			wantErrs:   2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			secrets, errs := AsSecrets(tc.namespace, tc.nameSuffix, nil)
			assert.Nil(t, secrets)
			assert.Len(t, errs, tc.wantErrs)

			secrets, errs = AsLegacySecrets(tc.namespace, tc.nameSuffix, nil)
			assert.Nil(t, secrets)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}
