// Copyright © 2018 Heptio
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

package cluster

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/heptio/contour/internal/dag"
)

type Visitor struct {
	*ClusterCache
	*dag.DAG

	clusters map[string]*v2.Cluster
}

func (v *Visitor) Visit() map[string]*v2.Cluster {
	v.clusters = make(map[string]*v2.Cluster)
	v.DAG.Visit(v.visit)
	return v.clusters
}

func (v *Visitor) visit(vertex dag.Vertex) {
	if service, ok := vertex.(*dag.Service); ok {
		v.edscluster(service)
	}
	// recurse into children of v
	vertex.Visit(v.visit)
}

func (v *Visitor) edscluster(svc *dag.Service) {
	name := hashname(60, svc.Namespace(), svc.Name(), strconv.Itoa(int(svc.Port)))
	if _, ok := v.clusters[name]; ok {
		// already created this cluster via another edge. skip it.
		return
	}

	c := &v2.Cluster{
		Name:             name,
		Type:             v2.Cluster_EDS,
		EdsClusterConfig: edsconfig("contour", servicename(svc.Namespace(), svc.Name(), svc.ServicePort.Name)),
		ConnectTimeout:   250 * time.Millisecond,
		LbPolicy:         v2.Cluster_ROUND_ROBIN,
	}

	/*
		thresholds := &cluster.CircuitBreakers_Thresholds{
			MaxConnections:     parseAnnotationUInt32(svc.Annotations, annotationMaxConnections),
			MaxPendingRequests: parseAnnotationUInt32(svc.Annotations, annotationMaxPendingRequests),
			MaxRequests:        parseAnnotationUInt32(svc.Annotations, annotationMaxRequests),
			MaxRetries:         parseAnnotationUInt32(svc.Annotations, annotationMaxRetries),
		}
		if thresholds.MaxConnections != nil || thresholds.MaxPendingRequests != nil ||
			thresholds.MaxRequests != nil || thresholds.MaxRetries != nil {
			c.CircuitBreakers = &cluster.CircuitBreakers{
				Thresholds: []*cluster.CircuitBreakers_Thresholds{
					thresholds,
				},
			}
		}
	*/

	switch svc.Protocol {
	case "h2":
		c.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
		c.TlsContext = &auth.UpstreamTlsContext{
			CommonTlsContext: &auth.CommonTlsContext{
				AlpnProtocols: []string{"h2"},
			},
		}
	case "h2c":
		c.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
	}
	v.clusters[c.Name] = c
}

func edsconfig(source, name string) *v2.Cluster_EdsClusterConfig {
	return &v2.Cluster_EdsClusterConfig{
		EdsConfig:   apiconfigsource(source), // hard coded by initconfig
		ServiceName: name,
	}
}

func apiconfigsource(clusters ...string) *core.ConfigSource {
	return &core.ConfigSource{
		ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
			ApiConfigSource: &core.ApiConfigSource{
				ApiType:      core.ApiConfigSource_GRPC,
				ClusterNames: clusters,
			},
		},
	}
}

// hashname takes a lenth l and a varargs of strings s and returns a string whose length
// which does not exceed l. Internally s is joined with strings.Join(s, "/"). If the
// combined length exceeds l then hashname truncates each element in s, starting from the
// end using a hash derived from the contents of s (not the current element). This process
// continues until the length of s does not exceed l, or all elements have been truncated.
// In which case, the entire string is replaced with a hash not exceeding the length of l.
func hashname(l int, s ...string) string {
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

func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}

// servicename returns a fixed name for this service and portname
func servicename(namespace, name, portname string) string {
	sn := []string{
		namespace,
		name,
		portname,
	}
	if portname == "" {
		sn = sn[:2]
	}
	return strings.Join(sn, "/")
}
