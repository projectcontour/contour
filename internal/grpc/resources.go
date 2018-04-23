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

// cache represents a source of proto.Message valus that can be registered
// for interest.
type cache interface {
	// Values returns a slice of proto.Message implementations that match
	// the provided filter.
	Values(func(string) bool) []proto.Message

	// Register registers ch to receive a value when Notify is called.
	Register(chan int, int)
}

// CDS implements the CDS v2 gRPC API.
type CDS struct {
	cache
}

// Values returns a sorted list of Clusters.
func (c *CDS) Values(filter func(string) bool) []proto.Message {
	v := c.cache.Values(filter)
	sort.Stable(clusterByName(v))
	return v
}

func (c *CDS) TypeURL() string { return clusterType }

type clusterByName []proto.Message

func (c clusterByName) Len() int           { return len(c) }
func (c clusterByName) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c clusterByName) Less(i, j int) bool { return c[i].(*v2.Cluster).Name < c[j].(*v2.Cluster).Name }

// EDS implements the EDS v2 gRPC API.
type EDS struct {
	cache
}

// Values returns a sorted list of ClusterLoadAssignments.
func (e *EDS) Values(filter func(string) bool) []proto.Message {
	v := e.cache.Values(filter)
	sort.Stable(clusterLoadAssignmentsByName(v))
	return v
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
	cache
}

// Values returns a sorted list of Listeners.
func (l *LDS) Values(filter func(string) bool) []proto.Message {
	v := l.cache.Values(filter)
	sort.Stable(listenersByName(v))
	return v
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
		// Values returns a slice of proto.Message implementations that match
		// the provided filter.
		Values(func(string) bool) []proto.Message
	}
	*contour.Cond
}

// Values returns a sorted list of RouteConfigurations.
func (r *RDS) Values(filter func(string) bool) []proto.Message {
	// TODO(dfc) avoid this expensive sort
	toRouteVirtualHosts := func(ms []proto.Message) []route.VirtualHost {
		r := make([]route.VirtualHost, 0, len(ms))
		for _, m := range ms {
			r = append(r, *(m.(*route.VirtualHost)))
		}
		sort.Stable(virtualHostsByName(r))
		return r
	}

	matchAll := func(string) bool { return true }
	return []proto.Message{
		&v2.RouteConfiguration{
			Name:         "ingress_http", // TODO(dfc) matches LDS configuration?
			VirtualHosts: toRouteVirtualHosts(r.HTTP.Values(matchAll)),
		},
		&v2.RouteConfiguration{

			Name:         "ingress_https", // TODO(dfc) matches LDS configuration?
			VirtualHosts: toRouteVirtualHosts(r.HTTPS.Values(matchAll)),
		},
	}
}

func (r *RDS) TypeURL() string { return routeType }

type virtualHostsByName []route.VirtualHost

func (v virtualHostsByName) Len() int           { return len(v) }
func (v virtualHostsByName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v virtualHostsByName) Less(i, j int) bool { return v[i].Name < v[j].Name }
