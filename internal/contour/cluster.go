// Copyright © 2017 Heptio
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

package contour

import (
	"strconv"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	v2cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"k8s.io/api/core/v1"
)

const (
	annotationMaxConnections     = "contour.heptio.com/max-connections"
	annotationMaxPendingRequests = "contour.heptio.com/max-pending-requests"
	annotationMaxRequests        = "contour.heptio.com/max-requests"
	annotationMaxRetries         = "contour.heptio.com/max-retries"
	annotationUpstreamProtocol   = "contour.heptio.com/upstream-protocol"
)

// ClusterCache manage the contents of the gRPC SDS cache.
type ClusterCache struct {
	clusterCache
	Cond
}

// recomputeService recomputes SDS cache entries, adding, updating, or removing
// entries as required.
// If oldsvc is nil, entries in newsvc are unconditionally added to the SDS cache.
// If oldsvc differs to newsvc, then the entries present only oldsvc will be removed from
// the SDS cache, present in newsvc will be added. If newsvc is nil, entries in oldsvc
// will be unconditionally deleted.
//
// Each CDS entry has a name, which is hashed according to the envoy 60 char limit, and
// a sevicename which is freeform. The servicename is how EDS and CDS locate each other.
// In order to avoid having to have access to the endpoints during service recomputation,
// and vice versa, the service name is generated blindly with prior knowlege of how the
// endpoint controller will write the endpoints record for EDS. In effect, all service
// names come in the form NAMESPACE / NAME / SERVICEPORT NAME. However SERVICEPORT NAME
// may be blank, and so both the SERVICEPORT NAME component and the preceeding slash may
// be elided in the case that there is a single, unnamed, service port in the spec.
func (cc *ClusterCache) recomputeService(oldsvc, newsvc *v1.Service) {
	if oldsvc == newsvc {
		// skip if oldsvc & newsvc == nil, or are the same object.
		return
	}

	defer cc.Notify()

	if oldsvc == nil {
		// if oldsvc is nil, replace it with a blank spec so entries
		// are unconditionally added.
		oldsvc = &v1.Service{
			ObjectMeta: newsvc.ObjectMeta,
		}
	}

	if newsvc == nil {
		// if newsvc is nil, replace it with a blank spec so entries
		// are unconditaionlly deleted.
		newsvc = &v1.Service{
			ObjectMeta: oldsvc.ObjectMeta,
		}
	}

	// parse upstream protocol annotations
	up := parseUpstreamProtocols(newsvc.Annotations, annotationUpstreamProtocol, "h2", "h2c")

	// iterate over all ports in newsvc adding or updating their records and
	// recording that face in named and unnamed.
	named := make(map[string]v1.ServicePort)
	unnamed := make(map[int32]v1.ServicePort)
	for _, p := range newsvc.Spec.Ports {
		switch p.Protocol {
		case "TCP":
			// eds needs a stable name to find this sds entry.
			// ideally we can generate this information from that recorded by the
			// endpoint controller in the endpoint record.
			// p.Name will be blank on the condition that there is a single serviceport
			// entry in this service spec.
			config := edsconfig("contour", servicename(newsvc.ObjectMeta, p.Name))
			if p.Name != "" {
				// service port is named, so we must generate both a cluster for the port name
				// and a cluster for the port number.
				c := edscluster(newsvc, p.Name, up[p.Name], config)
				cc.Add(c)
				// it is safe to use p.Name as the key because the API server enforces
				// the invariant that Name will only be blank if there is a single port
				// in the service spec. This there will only be one entry in the map,
				// { "": p }
				named[p.Name] = p
			}
			portString := strconv.Itoa(int(p.Port))
			c := edscluster(newsvc, portString, up[portString], config)
			cc.Add(c)
			unnamed[p.Port] = p
		default:
			// ignore UDP and other port types.
		}
	}

	// iterate over all the ports in oldsvc, if they are not found in named or unnamed then remove their
	// entires from the cache.
	for _, p := range oldsvc.Spec.Ports {
		switch p.Protocol {
		case "TCP":
			if _, found := named[p.Name]; !found {
				cc.Remove(hashname(60, oldsvc.ObjectMeta.Namespace, oldsvc.ObjectMeta.Name, p.Name))
			}
			if _, found := unnamed[p.Port]; !found {
				cc.Remove(hashname(60, oldsvc.ObjectMeta.Namespace, oldsvc.ObjectMeta.Name, strconv.Itoa(int(p.Port))))
			}
		default:
			// ignore UDP and other port types.
		}
	}
}

func edscluster(svc *v1.Service, portString, upstreamProtocol string, config *v2.Cluster_EdsClusterConfig) *v2.Cluster {
	cluster := &v2.Cluster{
		Name:             hashname(60, svc.ObjectMeta.Namespace, svc.ObjectMeta.Name, portString),
		Type:             v2.Cluster_EDS,
		EdsClusterConfig: config,
		ConnectTimeout:   250 * time.Millisecond,
		LbPolicy:         v2.Cluster_ROUND_ROBIN,
	}

	thresholds := &v2cluster.CircuitBreakers_Thresholds{
		MaxConnections:     parseAnnotationUInt32(svc.Annotations, annotationMaxConnections),
		MaxPendingRequests: parseAnnotationUInt32(svc.Annotations, annotationMaxPendingRequests),
		MaxRequests:        parseAnnotationUInt32(svc.Annotations, annotationMaxRequests),
		MaxRetries:         parseAnnotationUInt32(svc.Annotations, annotationMaxRetries),
	}
	if thresholds.MaxConnections != nil || thresholds.MaxPendingRequests != nil ||
		thresholds.MaxRequests != nil || thresholds.MaxRetries != nil {
		cluster.CircuitBreakers = &v2cluster.CircuitBreakers{Thresholds: []*v2cluster.CircuitBreakers_Thresholds{thresholds}}
	}

	switch upstreamProtocol {
	case "h2":
		cluster.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
		cluster.TlsContext = &auth.UpstreamTlsContext{
			CommonTlsContext: &auth.CommonTlsContext{
				AlpnProtocols: []string{"h2"},
			},
		}
	case "h2c":
		cluster.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
	}

	return cluster
}

func edsconfig(source, name string) *v2.Cluster_EdsClusterConfig {
	return &v2.Cluster_EdsClusterConfig{
		EdsConfig:   apiconfigsource(source), // hard coded by initconfig
		ServiceName: name,
	}
}
