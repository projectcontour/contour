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

package httpproxy

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func TestHTTPProxy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTPProxy tests")
}

var _ = Describe("HTTPProxy", func() {
	var f *e2e.Framework

	BeforeEach(func() {
		f = e2e.NewFramework(GinkgoT())
	})

	It("002-header-condition-match", func() {
		testHeaderConditionMatch(f)
	})
	It("003-path-condition-match", func() {
		testPathConditionMatch(f)
	})
	It("004-https-sni-enforcement", func() {
		testHTTPSSNIEnforcement(f)
	})
	It("005-pod-restart", func() {
		testPodRestart(f)
	})
	It("006-merge-slash", func() {
		testMergeSlash(f)
	})
	It("008-tcproute-https-termination", func() {
		testTCPRouteHTTPSTermination(f)
	})
	It("009-https-misdirected-request", func() {
		testHTTPSMisdirectedRequest(f)
	})
	It("010-include-prefix-condition", func() {
		testIncludePrefixCondition(f)
	})
	It("012-https-fallback-certificate", func() {
		testHTTPSFallbackCertificate(f)
	})
})

// httpProxyValid returns true if the proxy has a .status.currentStatus
// of "valid".
func httpProxyValid(proxy *contourv1.HTTPProxy) bool {
	return proxy != nil && proxy.Status.CurrentStatus == "valid"
}
