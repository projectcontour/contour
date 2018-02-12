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

	v2 "github.com/envoyproxy/go-control-plane/api"
	"k8s.io/api/core/v1"
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
			sname := p.Name
			if sname == "" {
				// if this port is unnamed, then there is only one service port
				// for this service.
				sname = strconv.Itoa(int(p.Port))
			}
			config := edsconfig("contour", servicename(newsvc.ObjectMeta, sname))

			// sname is the entry that EDS will try to match on, it is independant
			// of named and unnamed ports below.
			if p.Name != "" {
				// service port is named, so we must generate both a cluster for the port name
				// and a cluster for the port number.
				c := edscluster(hashname(60, newsvc.ObjectMeta.Namespace, newsvc.ObjectMeta.Name, p.Name), config)
				cc.Add(c)
				// it is safe to use p.Name as the key because the API server enforces
				// the invariant that Name will only be blank if there is a single port
				// in the service spec. This there will only be one entry in the map,
				// { "": p }
				named[p.Name] = p
			}
			c := edscluster(hashname(60, newsvc.ObjectMeta.Namespace, newsvc.ObjectMeta.Name, strconv.Itoa(int(p.Port))), config)
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

func edscluster(name string, config *v2.Cluster_EdsClusterConfig) *v2.Cluster {
	return &v2.Cluster{
		Name:             name,
		Type:             v2.Cluster_EDS,
		EdsClusterConfig: config,
		ConnectTimeout:   250 * time.Millisecond,
		LbPolicy:         v2.Cluster_ROUND_ROBIN,
	}
}

func edsconfig(source, name string) *v2.Cluster_EdsClusterConfig {
	return &v2.Cluster_EdsClusterConfig{
		EdsConfig:   apiconfigsource(source), // hard coded by initconfig
		ServiceName: name,
	}
}
