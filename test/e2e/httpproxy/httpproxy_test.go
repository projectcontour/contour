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
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

func TestHTTPProxy(t *testing.T) {
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
	It("014-external-auth", func() {
		testExternalAuth(f)
	})
	It("015-http-health-checks", func() {
		testHTTPHealthChecks(f)
	})
	It("016-dynamic-headers", func() {
		testDynamicHeaders(f)
	})
	It("017-host-header-rewrite", func() {
		testHostHeaderRewrite(f)
	})
	It("019-local-rate-limiting-vhost", func() {
		testLocalRateLimitingVirtualHost(f)
	})
	It("019-local-rate-limiting-route", func() {
		testLocalRateLimitingRoute(f)
	})
	It("020-global-rate-limiting-vhost-non-tls", func() {
		testGlobalRateLimitingVirtualHostNonTLS(f)
	})
	It("020-global-rate-limiting-route-non-tls", func() {
		testGlobalRateLimitingRouteNonTLS(f)
	})
	It("020-global-rate-limiting-vhost-tls", func() {
		testGlobalRateLimitingVirtualHostTLS(f)
	})
	It("020-global-rate-limiting-route-tls", func() {
		testGlobalRateLimitingRouteTLS(f)
	})
})

// httpProxyValid returns true if the proxy has a .status.currentStatus
// of "valid".
func httpProxyValid(proxy *contourv1.HTTPProxy) bool {
	return proxy != nil && proxy.Status.CurrentStatus == "valid"
}
