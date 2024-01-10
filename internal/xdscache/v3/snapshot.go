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
	"reflect"

	envoy_types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoy_cache_v3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoy_resource_v3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/google/uuid"
	"github.com/projectcontour/contour/internal/dag"
	contour_xds_v3 "github.com/projectcontour/contour/internal/xds/v3"
	"github.com/projectcontour/contour/internal/xdscache"
	"github.com/sirupsen/logrus"
)

// SnapshotHandler responds to DAG builds via the OnChange()
// event and generates and caches go-control-plane Snapshots.
type SnapshotHandler struct {
	// SnapshotCache contains go-control-plane Snapshots
	// and is used by the go-control-plane xDS server.
	SnapshotCache envoy_cache_v3.SnapshotCache

	// resources contains the Contour xDS resource caches.
	resources map[envoy_resource_v3.Type]xdscache.ResourceCache
	log       logrus.FieldLogger
}

// NewSnapshotHandler returns an instance of SnapshotHandler.
func NewSnapshotHandler(resources []xdscache.ResourceCache, snapshotCache envoy_cache_v3.SnapshotCache, logger logrus.FieldLogger) *SnapshotHandler {
	return &SnapshotHandler{
		resources:     parseResources(resources),
		SnapshotCache: snapshotCache,
		log:           logger,
	}
}

// Refresh is called when the EndpointsTranslator updates values
// in its cache.
func (s *SnapshotHandler) Refresh() {
	s.generateNewSnapshot()
}

// OnChange is called when the DAG is rebuilt and a new snapshot is needed.
func (s *SnapshotHandler) OnChange(*dag.DAG) {
	s.generateNewSnapshot()
}

// generateNewSnapshot creates and caches a new go-control-plane
// Snapshot based on the contents of the Contour xDS resource caches.
func (s *SnapshotHandler) generateNewSnapshot() {
	// Generate new snapshot version.
	version := uuid.NewString()

	// Convert caches to envoy xDS Resources.
	resources := map[envoy_resource_v3.Type][]envoy_types.Resource{
		envoy_resource_v3.EndpointType: asResources(s.resources[envoy_resource_v3.EndpointType].Contents()),
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

	if err := s.SnapshotCache.SetSnapshot(context.Background(), contour_xds_v3.Hash.String(), snapshot); err != nil {
		s.log.Errorf("failed to store snapshot version %q: %s", version, err)
		return
	}
}

// asResources casts the given slice of values (that implement the envoy_types.Resource
// interface) to a slice of envoy_types.Resource. If the length of the slice is 0, it
// returns nil.
func asResources(messages any) []envoy_types.Resource {
	v := reflect.ValueOf(messages)
	if v.Len() == 0 {
		return nil
	}

	protos := make([]envoy_types.Resource, v.Len())

	for i := range protos {
		protos[i] = v.Index(i).Interface().(envoy_types.Resource)
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
