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
	"errors"
	"fmt"
	"testing"

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"
)

func TestIsValidSecret(t *testing.T) {
	tests := map[string]struct {
		cert, key string
		valid     bool
		err       error
	}{
		"normal": {
			cert:  fixture.CERTIFICATE,
			key:   fixture.RSA_PRIVATE_KEY,
			valid: true,
			err:   nil,
		},
		"missing CN": {
			cert:  fixture.MISSING_CN_CERT,
			key:   fixture.MISSING_CN_KEY,
			valid: false,
			err:   errors.New("invalid TLS certificate: certificate has no common name or subject alt name"),
		},
		"EC cert with SubjectAltName only": {
			cert:  fixture.EC_CERTIFICATE,
			key:   fixture.EC_PRIVATE_KEY,
			valid: true,
			err:   nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			type Result struct {
				Valid bool
				Err   error
			}

			want := Result{Valid: tc.valid, Err: tc.err}

			valid, err := isValidSecret(&core_v1.Secret{
				// objectmeta omitted
				Type: core_v1.SecretTypeTLS,
				Data: secretdata(tc.cert, tc.key),
			})
			got := Result{Valid: valid, Err: err}

			assert.Equal(t, want, got)
		})
	}
}

func secretdata(cert, key string) map[string][]byte {
	return map[string][]byte{
		core_v1.TLSCertKey:       []byte(cert),
		core_v1.TLSPrivateKeyKey: []byte(key),
	}
}

// caBundleData returns a CA certificate bundle map whose value is
// the given set of PEM certificates intermingled with some non-PEM
// data.
//
// See also: https://tools.ietf.org/html/rfc7468#section-5.2
func caBundleData(cert ...string) map[string][]byte {
	var data string

	data += "start of CA bundle\n"

	for n, c := range cert {
		data += fmt.Sprintf("certificate %d\n", n)
		data += c
		data += "\n"
	}

	data += "end of CA bundle\n"

	return map[string][]byte{
		CACertificateKey: []byte(data),
	}
}
