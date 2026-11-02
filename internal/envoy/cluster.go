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

package envoy

import (
	"crypto/sha1" // nolint:gosec
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	"github.com/projectcontour/contour/internal/dag"
)

// Clustername returns the name of the CDS cluster for this service.
//
// Note: Cluster name is used to deduplicate clusters.
// When for example two HTTPProxies result in Clusters with equal name, only single cluster will be configured to Envoy.
// Therefore the generated name must contain all relevant fields that make the cluster unique.
func Clustername(cluster *dag.Cluster) string {
	service := cluster.Upstream
	buf := cluster.LoadBalancerPolicy
	if hc := cluster.HTTPHealthCheckPolicy; hc != nil {
		if hc.Timeout > 0 {
			buf += hc.Timeout.String()
		}
		if hc.Interval > 0 {
			buf += hc.Interval.String()
		}
		if hc.UnhealthyThreshold > 0 {
			buf += strconv.Itoa(int(hc.UnhealthyThreshold))
		}
		if hc.HealthyThreshold > 0 {
			buf += strconv.Itoa(int(hc.HealthyThreshold))
		}
		buf += hc.Path
	}
	if uv := cluster.UpstreamValidation; uv != nil {
		if len(uv.CACertificates) > 0 {
			buf += uv.CACertificates[0].Object.Name
		}
		if len(uv.SubjectNames) > 0 {
			buf += uv.SubjectNames[0]
		}
	}
	buf += cluster.Protocol + cluster.SNI
	if !cluster.TimeoutPolicy.IdleConnectionTimeout.UseDefault() {
		buf += cluster.TimeoutPolicy.IdleConnectionTimeout.Duration().String()
	}
	if cluster.SlowStartConfig != nil {
		buf += cluster.SlowStartConfig.String()
	}

	// This isn't a crypto hash, we just want a unique name.
	hash := sha1.Sum([]byte(buf)) // nolint:gosec

	ns := service.Weighted.ServiceNamespace
	name := service.Weighted.ServiceName
	return Hashname(60, ns, name, strconv.Itoa(int(service.Weighted.ServicePort.Port)), fmt.Sprintf("%x", hash[:5]))
}

// AltStatName generates an alternative stat name for the service
// using format ns_name_port
func AltStatName(service *dag.Service) string {
	parts := []string{service.Weighted.ServiceNamespace, service.Weighted.ServiceName, strconv.Itoa(int(service.Weighted.ServicePort.Port))}
	return strings.Join(parts, "_")
}

// Hashname takes a length l and a varargs of strings s and returns a string whose length
// which does not exceed l. Internally s is joined with strings.Join(s, "/"). If the
// combined length exceeds l then hashname truncates each element in s, starting from the
// end using a hash derived from the contents of s (not the current element). This process
// continues until the length of s does not exceed l, or all elements have been truncated.
// In which case, the entire string is replaced with a hash not exceeding the length of l.
func Hashname(l int, s ...string) string {
	const shorthash = 6 // the length of the shorthash

	r := strings.Join(s, "/")
	if l > len(r) {
		// we're under the limit, nothing to do
		return r
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r)))
	for n := len(s) - 1; n >= 0; n-- {
		s[n] = truncate(l/len(s), s[n], hash[:shorthash])
		r = strings.Join(s, "/")
		if l > len(r) {
			return r
		}
	}
	// truncated everything, but we're still too long
	// just return the hash truncated to l.
	return hash[:min(len(hash), l)]
}

// truncate truncates s to l length by replacing the
// end of s with -suffix.
func truncate(l int, s, suffix string) string {
	if l >= len(s) {
		// under the limit, nothing to do
		return s
	}
	if l <= len(suffix) {
		// easy case, just return the start of the suffix
		return suffix[:min(l, len(suffix))]
	}
	return s[:l-len(suffix)-1] + "-" + suffix
}

// AnyPositive indicates if any of the values provided are greater than zero.
func AnyPositive(first uint32, rest ...uint32) bool {
	if first > 0 {
		return true
	}
	for _, v := range rest {
		if v > 0 {
			return true
		}
	}
	return false
}

// DNSNameClusterName returns the name of the CDS cluster for this DNS name based upstream (e.g. JWKS endpoint).
// The name looks like dnsname/https/idp.example.com/443/92e2d87ab4868b426840.
//
// Note: Cluster name is used to deduplicate clusters.
// When for example two HTTPProxies result in Clusters with equal name, only single cluster will be configured to Envoy.
// Therefore the generated name must contain all relevant fields that make the cluster unique.
func DNSNameClusterName(cluster *dag.DNSNameCluster) string {
	// Values are quoted to keep their boundaries unambiguous, otherwise e.g. CA secrets
	// tenant-a/idp-ca and tenant-ai/dp-ca would both become "tenant-aidp-ca".
	buf := strconv.Quote(cluster.Scheme) + strconv.Quote(cluster.Address) + strconv.Quote(strconv.Itoa(cluster.Port))
	buf += strconv.Quote(cluster.DNSLookupFamily)

	if uv := cluster.UpstreamValidation; uv != nil {
		for _, ca := range uv.CACertificates {
			if ca != nil {
				buf += strconv.Quote(ca.Namespace()) + strconv.Quote(ca.Name())
			}
		}
		for _, sn := range uv.SubjectNames {
			buf += strconv.Quote(sn)
		}
	}

	// Unlike in Clustername(), the name does not include the namespace, so a user in
	// another namespace could use the same JWKS URI and try subjectNames until the hash
	// matches. The hash must be long enough to make that infeasible.
	hash := sha256.Sum256([]byte(buf))

	// Scheme, address and port are added as plain text to make the name readable.
	// Hashname() is not used since it would truncate long hostnames.
	return strings.Join([]string{"dnsname", cluster.Scheme, cluster.Address, strconv.Itoa(cluster.Port), fmt.Sprintf("%x", hash[:10])}, "/")
}
