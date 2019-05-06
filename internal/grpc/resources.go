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
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/pkg/cache"

	"github.com/gogo/protobuf/proto"
)

const (
	endpointType = cache.EndpointType
	clusterType  = cache.ClusterType
	routeType    = cache.RouteType
	listenerType = cache.ListenerType
	secretType   = cache.SecretType
)

// cache represents a source of proto.Message valus that can be registered
// for interest.
type Cache interface {
	// Values returns a slice of proto.Message implementations that match
	// the provided filter.
	Values(func(string) bool) []proto.Message

	// Register registers ch to receive a value when Notify is called.
	Register(chan int, int)
}

// CDS implements the CDS v2 gRPC API.
type CDS struct {
	Cache
}

// Values returns a sorted list of Clusters.
func (c *CDS) Values(filter func(string) bool) []proto.Message {
	v := c.Cache.Values(filter)
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
	Cache
}

// Values returns a sorted list of ClusterLoadAssignments.
func (e *EDS) Values(filter func(string) bool) []proto.Message {
	v := e.Cache.Values(filter)
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
	Cache
}

// Values returns a sorted list of Listeners.
func (l *LDS) Values(filter func(string) bool) []proto.Message {
	v := l.Cache.Values(filter)
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
	Cache
}

// Values returns a sorted list of RouteConfigurations.
func (r *RDS) Values(filter func(string) bool) []proto.Message {
	v := r.Cache.Values(filter)
	sort.Stable(routeConfigurationsByName(v))
	return v
}

func (r *RDS) TypeURL() string { return routeType }

type routeConfigurationsByName []proto.Message

func (r routeConfigurationsByName) Len() int      { return len(r) }
func (r routeConfigurationsByName) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r routeConfigurationsByName) Less(i, j int) bool {
	return r[i].(*v2.RouteConfiguration).Name < r[j].(*v2.RouteConfiguration).Name
}

// SDS implements the RDS v2 gRPC API.
type SDS struct {
	Cache
}

// Values returns a sorted list of RouteConfigurations.
func (s *SDS) Values(filter func(string) bool) []proto.Message {
	v := s.Cache.Values(filter)
	sort.Stable(secretsByName(v))
	return v
}

func (s *SDS) TypeURL() string { return secretType }

type secretsByName []proto.Message

func (s secretsByName) Len() int      { return len(s) }
func (s secretsByName) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s secretsByName) Less(i, j int) bool {
	return s[i].(*auth.Secret).Name < s[j].(*auth.Secret).Name
}
