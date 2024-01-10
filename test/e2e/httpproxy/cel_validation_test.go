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
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testCELValidation(namespace string) {
	Specify("UpstreamValidation is validated by CEL rule on creation", func() {
		t := f.T()

		subjectNameNoMatch := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "subjectname-no-match",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "example.com",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "any-service-name",
								Port: 80000,
								UpstreamValidation: &contourv1.UpstreamValidation{
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
