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
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestVirtualHostValid(t *testing.T) {

	vh := VirtualHost{}
	assert.False(t, vh.Valid())

	vh = VirtualHost{
		routes: map[string]*Route{
			"/": {},
		},
	}
	assert.True(t, vh.Valid())
}

func TestSecureVirtualHostValid(t *testing.T) {

	vh := SecureVirtualHost{}
	assert.False(t, vh.Valid())

	vh = SecureVirtualHost{
		Secret: new(Secret),
	}
	assert.False(t, vh.Valid())

	vh = SecureVirtualHost{
		VirtualHost: VirtualHost{
			routes: map[string]*Route{
				"/": {},
			},
		},
	}
	assert.False(t, vh.Valid())

	vh = SecureVirtualHost{
		Secret: new(Secret),
		VirtualHost: VirtualHost{
			routes: map[string]*Route{
				"/": {},
			},
		},
	}
	assert.True(t, vh.Valid())

	vh = SecureVirtualHost{
		TCPProxy: new(TCPProxy),
	}
	assert.True(t, vh.Valid())

	vh = SecureVirtualHost{
		Secret:   new(Secret),
		TCPProxy: new(TCPProxy),
	}
	assert.True(t, vh.Valid())
}

func TestPeerValidationContext(t *testing.T) {
	pvc1 := PeerValidationContext{
		CACertificate: &Secret{
			Object: &v1.Secret{
				Data: map[string][]byte{
					CACertificateKey: []byte("cacert"),
				},
			},
		},
		SubjectName: "subject",
	}
	pvc2 := PeerValidationContext{}
	var pvc3 *PeerValidationContext

	assert.Equal(t, pvc1.GetSubjectName(), "subject")
	assert.Equal(t, pvc1.GetCACertificate(), []byte("cacert"))
	assert.Equal(t, pvc2.GetSubjectName(), "")
	assert.Equal(t, pvc2.GetCACertificate(), []byte(nil))
	assert.Equal(t, pvc3.GetSubjectName(), "")
	assert.Equal(t, pvc3.GetCACertificate(), []byte(nil))
}

func TestObserverFunc(t *testing.T) {
	// Ensure nil doesn't panic.
	ObserverFunc(nil).OnChange(nil)

	// Ensure the given function gets called.
	result := false
	ObserverFunc(func(*DAG) { result = true }).OnChange(nil)
	assert.Equal(t, true, result)
}
