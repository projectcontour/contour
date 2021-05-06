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

// +build e2e

package ingress

import (
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
)

func TestIngress(t *testing.T) {
	RunSpecs(t, "Ingress tests")
}

var _ = Describe("Ingress", func() {
	var f *e2e.Framework

	BeforeEach(func() {
		f = e2e.NewFramework(GinkgoT())
	})

	It("002-ingress-ensure-v1beta1", func() {
		testEnsureV1Beta1(f)
	})
})
