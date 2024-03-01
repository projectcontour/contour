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

package v3

import (
	"context"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoy_cache_v3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoy_resource_v3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	"github.com/projectcontour/contour/internal/dag"
	contour_xds_v3 "github.com/projectcontour/contour/internal/xds/v3"
	"github.com/projectcontour/contour/internal/xdscache"
)

// SnapshotHandler responds to DAG builds via the OnChange()
// event and Endpoint updates via the Refresh() event and
// generates and caches go-control-plane Snapshots.
type SnapshotHandler struct {
	resources     map[envoy_resource_v3.Type]xdscache.ResourceCache
	snapshotCache envoy_cache_v3.SnapshotCache
	edsCache      *envoy_cache_v3.LinearCache
	muxCache      *envoy_cache_v3.MuxCache
	log           logrus.FieldLogger
}

// NewSnapshotHandler returns an instance of SnapshotHandler.
func NewSnapshotHandler(resources []xdscache.ResourceCache, log logrus.FieldLogger) *SnapshotHandler {
	var (
		snapshotCache = envoy_cache_v3.NewSnapshotCache(false, &contour_xds_v3.Hash, log.WithField("context", "snapshotCache"))
		edsCache      = envoy_cache_v3.NewLinearCache(envoy_resource_v3.EndpointType, envoy_cache_v3.WithLogger(log.WithField("context", "edsCache")))

		muxCache = &envoy_cache_v3.MuxCache{
			Caches: map[string]envoy_cache_v3.Cache{
				envoy_resource_v3.ListenerType: snapshotCache,
				envoy_resource_v3.ClusterType:  snapshotCache,
				envoy_resource_v3.RouteType:    snapshotCache,
				envoy_resource_v3.SecretType:   snapshotCache,
				envoy_resource_v3.RuntimeType:  snapshotCache,
				envoy_resource_v3.EndpointType: edsCache,
			},
			Classify: func(req *envoy_service_discovery_v3.DiscoveryRequest) string {
				return req.GetTypeUrl()
			},
			ClassifyDelta: func(dr *envoy_cache_v3.DeltaRequest) string {
				return dr.GetTypeUrl()
			},
		}
	)

	return &SnapshotHandler{
		resources:     parseResources(resources),
		snapshotCache: snapshotCache,
		edsCache:      edsCache,
		muxCache:      muxCache,
		log:           log,
	}
}

func (s *SnapshotHandler) GetCache() envoy_cache_v3.Cache {
	return s.muxCache
}

// Refresh is called when the EndpointsTranslator updates values
// in its cache. It updates the ClusterLoadAssignments linear cache.
func (s *SnapshotHandler) Refresh() {
	endpoints := s.resources[envoy_resource_v3.EndpointType].ContentsByName()

	resources := make(map[string]envoy_types.Resource, len(endpoints))
	for name, val := range endpoints {
		resources[name] = val
	}

	s.edsCache.SetResources(resources)
}

// OnChange is called when the DAG is rebuilt and a new snapshot is needed.
// It creates and caches a new go-control-plane Snapshot based on the
// contents of the Contour xDS resource caches.
func (s *SnapshotHandler) OnChange(*dag.DAG) {
	// Generate new snapshot version.
	version := uuid.NewString()

	// Convert caches to envoy xDS Resources.
	resources := map[envoy_resource_v3.Type][]envoy_types.Resource{
		envoy_resource_v3.ClusterType:  asResources(s.resources[envoy_resource_v3.ClusterType].Contents()),
		envoy_resource_v3.RouteType:    asResources(s.resources[envoy_resource_v3.RouteType].Contents()),
		envoy_resource_v3.ListenerType: asResources(s.resources[envoy_resource_v3.ListenerType].Contents()),
		envoy_resource_v3.SecretType:   asResources(s.resources[envoy_resource_v3.SecretType].Contents()),
		envoy_resource_v3.RuntimeType:  asResources(s.resources[envoy_resource_v3.RuntimeType].Contents()),
	}

	snapshot, err := envoy_cache_v3.NewSnapshot(version, resources)
	if err != nil {
		s.log.Errorf("failed to generate snapshot version %q: %s", version, err)
		return
	}

	if err := s.snapshotCache.SetSnapshot(context.Background(), contour_xds_v3.Hash.String(), snapshot); err != nil {
		s.log.Errorf("failed to store snapshot version %q: %s", version, err)
		return
	}
}

// asResources converts the given slice of values (that implement the envoy_types.Resource
// interface) to a slice of envoy_types.Resource. If the length of the slice is 0, it
// returns nil.
func asResources[T proto.Message](messages []T) []envoy_types.Resource {
	if len(messages) == 0 {
		return nil
	}

	protos := make([]envoy_types.Resource, len(messages))

	for i, resource := range messages {
		protos[i] = resource
	}

	return protos
}

// parseResources converts an []ResourceCache to a map[envoy_types.ResponseType]ResourceCache
// for faster indexing when creating new snapshots.
func parseResources(resources []xdscache.ResourceCache) map[envoy_resource_v3.Type]xdscache.ResourceCache {
	resourceMap := make(map[envoy_resource_v3.Type]xdscache.ResourceCache, len(resources))

	for _, r := range resources {
		resourceMap[r.TypeURL()] = r
	}
	return resourceMap
}
