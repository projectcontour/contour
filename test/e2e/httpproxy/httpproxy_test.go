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

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

// subtests defines the tests to run as part of the HTTPProxy
// suite.
var subtests = map[string]func(t *testing.T, f *e2e.Framework){
	"002-header-condition-match":             testHeaderConditionMatch,
	"003-path-condition-match":               testPathConditionMatch,
	"004-https-sni-enforcement":              testHTTPSSNIEnforcement,
	"005-pod-restart":                        testPodRestart,
	"006-merge-slash":                        testMergeSlash,
	"008-tcp-route-https-termination":        testTCPRouteHTTPSTermination,
	"009-https-misdirected-request":          testHTTPSMisdirectedRequest,
	"010-include-prefix-condition":           testIncludePrefixCondition,
	"012-https-fallback-certificate":         testHTTPSFallbackCertificate,
	"016-dynamic-headers":                    testDynamicHeaders,
	"017-host-header-rewrite":                testHostHeaderRewrite,
	"018-external-name-service-insecure":     testExternalNameServiceInsecure,
	"019-local-rate-limiting-vhost":          testLocalRateLimitingVirtualHost,
	"019-local-rate-limiting-route":          testLocalRateLimitingRoute,
	"020-global-rate-limiting-vhost-non-tls": testGlobalRateLimitingVirtualHostNonTLS,
	"020-global-rate-limiting-route-non-tls": testGlobalRateLimitingRouteNonTLS,
	"020-global-rate-limiting-vhost-tls":     testGlobalRateLimitingVirtualHostTLS,
	"020-global-rate-limiting-route-tls":     testGlobalRateLimitingRouteTLS,
}

func TestHTTPProxy(t *testing.T) {
	e2e.NewFramework(t).RunParallel("group", subtests)
}

// httpProxyValid returns true if the proxy has a .status.currentStatus
// of "valid".
func httpProxyValid(proxy *contourv1.HTTPProxy) bool {
	return proxy != nil && proxy.Status.CurrentStatus == "valid"
}
