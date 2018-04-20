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

package grpc

import (
	"sort"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/contour"
)

// Resource types in xDS v2.
const (
	googleApis   = "type.googleapis.com/"
	typePrefix   = googleApis + "envoy.api.v2."
	endpointType = typePrefix + "ClusterLoadAssignment"
	clusterType  = typePrefix + "Cluster"
	routeType    = typePrefix + "RouteConfiguration"
	listenerType = typePrefix + "Listener"
)

// CDS implements the CDS v2 gRPC API.
type CDS struct {
	ClusterCache
}

// Resources returns the contents of CDS"s cache as a []types.Any.
// TODO(dfc) cache the results of Resources in the ClusterCache so
// we can avoid the error handling.
func (c *CDS) Resources() ([]types.Any, error) {
	v := c.Values()
	sort.Stable(clusterByName(v))
	resources := make([]types.Any, len(v))
	for i := range v {
		value, err := proto.Marshal(v[i])
		if err != nil {
			return nil, err
		}
		resources[i] = types.Any{TypeUrl: c.TypeURL(), Value: value}
	}
	return resources, nil
}

func (c *CDS) TypeURL() string { return clusterType }

type clusterByName []proto.Message

func (c clusterByName) Len() int           { return len(c) }
func (c clusterByName) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c clusterByName) Less(i, j int) bool { return c[i].(*v2.Cluster).Name < c[j].(*v2.Cluster).Name }

// EDS implements the EDS v2 gRPC API.
type EDS struct {
	ClusterLoadAssignmentCache
}

// Resources returns the contents of EDS"s cache as a []types.Any.
// TODO(dfc) cache the results of Resources in the ClusterLoadAssignmentCache so
// we can avoid the error handling.
func (e *EDS) Resources() ([]types.Any, error) {
	v := e.Values()
	sort.Stable(clusterLoadAssignmentsByName(v))
	resources := make([]types.Any, len(v))
	for i := range v {
		value, err := proto.Marshal(v[i])
		if err != nil {
			return nil, err
		}
		resources[i] = types.Any{TypeUrl: e.TypeURL(), Value: value}
	}
	return resources, nil
}

func (e *EDS) TypeURL() string { return endpointType }

type clusterLoadAssignmentsByName []proto.Message

func (c clusterLoadAssignmentsByName) Len() int      { return len(c) }
func (c clusterLoadAssignmentsByName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c clusterLoadAssignmentsByName) Less(i, j int) bool {
	return c[i].(*v2.ClusterLoadAssignment).ClusterName < c[j].(*v2.ClusterLoadAssignment).ClusterName
}

// LDS implements the LDS v2 gRPC API.
type LDS struct {
	ListenerCache
}

// Resources returns the contents of LDS"s cache as a []types.Any.
// TODO(dfc) cache the results of Resources in the ListenerCache so
// we can avoid the error handling.
func (l *LDS) Resources() ([]types.Any, error) {
	v := l.Values()
	sort.Stable(listenersByName(v))
	resources := make([]types.Any, len(v))
	for i := range v {
		value, err := proto.Marshal(v[i])
		if err != nil {
			return nil, err
		}
		resources[i] = types.Any{TypeUrl: l.TypeURL(), Value: value}
	}
	return resources, nil
}

func (l *LDS) TypeURL() string { return listenerType }

type listenersByName []proto.Message

func (l listenersByName) Len() int      { return len(l) }
func (l listenersByName) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l listenersByName) Less(i, j int) bool {
	return l[i].(*v2.Listener).Name < l[j].(*v2.Listener).Name
}

// RDS implements the RDS v2 gRPC API.
type RDS struct {
	HTTP, HTTPS interface {
		// Values returns a copy of the contents of the cache.
		// The slice and its contents should be treated as read-only.
		Values() []proto.Message
	}
	*contour.Cond
}

// Resources returns the contents of RDS"s cache as a []types.Any.
// TODO(dfc) cache the results of Resources in the VirtualHostCache so
// we can avoid the error handling.
func (r *RDS) Resources() ([]types.Any, error) {
	// TODO(dfc) avoid this expensive
	toRouteVirtualHosts := func(ms []proto.Message) []route.VirtualHost {
		r := make([]route.VirtualHost, 0, len(ms))
		for _, m := range ms {
			r = append(r, *(m.(*route.VirtualHost)))
		}
		sort.Stable(virtualHostsByName(r))
		return r
	}

	ingress_http, err := proto.Marshal(&v2.RouteConfiguration{
		Name:         "ingress_http", // TODO(dfc) matches LDS configuration?
		VirtualHosts: toRouteVirtualHosts(r.HTTP.Values()),
	})
	if err != nil {
		return nil, err
	}
	ingress_https, err := proto.Marshal(&v2.RouteConfiguration{

		Name:         "ingress_https", // TODO(dfc) matches LDS configuration?
		VirtualHosts: toRouteVirtualHosts(r.HTTPS.Values()),
	})
	if err != nil {
		return nil, err
	}
	return []types.Any{{
		TypeUrl: r.TypeURL(), Value: ingress_http,
	}, {
		TypeUrl: r.TypeURL(), Value: ingress_https,
	}}, nil
}

func (r *RDS) TypeURL() string { return routeType }

type virtualHostsByName []route.VirtualHost

func (v virtualHostsByName) Len() int           { return len(v) }
func (v virtualHostsByName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v virtualHostsByName) Less(i, j int) bool { return v[i].Name < v[j].Name }
