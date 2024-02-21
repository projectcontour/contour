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

//go:build e2e

package httpproxy

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func testCELValidation(namespace string) {
	Specify("UpstreamValidation is validated by CEL rule on creation", func() {
		t := f.T()

		subjectNameNoMatch := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "subjectname-no-match",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "example.com",
				},
				Routes: []contour_v1.Route{
					{
						Services: []contour_v1.Service{
							{
								Name: "any-service-name",
								Port: 80000,
								UpstreamValidation: &contour_v1.UpstreamValidation{
									CACertificate: "namespace/name",
									SubjectNames:  []string{"wrong.com", "example.com"},
									SubjectName:   "example.com",
								},
							},
						},
					},
				},
			},
		}

		err := f.Client.Create(context.TODO(), subjectNameNoMatch)
		require.Error(t, err)

		isExpectedErr := func(err error) bool {
			return strings.Contains(err.Error(), "subjectNames[0] must equal subjectName if set")
		}
		assert.True(t, isExpectedErr(err))
	})
}
