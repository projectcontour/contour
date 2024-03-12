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
	resources    map[envoy_resource_v3.Type]xdscache.ResourceCache
	defaultCache envoy_cache_v3.SnapshotCache
	edsCache     envoy_cache_v3.SnapshotCache
	mux          *envoy_cache_v3.MuxCache
	log          logrus.FieldLogger
}

// NewSnapshotHandler returns an instance of SnapshotHandler.
func NewSnapshotHandler(resources []xdscache.ResourceCache, log logrus.FieldLogger) *SnapshotHandler {
	var (
		defaultCache = envoy_cache_v3.NewSnapshotCache(false, &contour_xds_v3.Hash, log.WithField("context", "defaultCache"))
		edsCache     = envoy_cache_v3.NewSnapshotCache(false, &contour_xds_v3.Hash, log.WithField("context", "edsCache"))

		mux = &envoy_cache_v3.MuxCache{
			Caches: map[string]envoy_cache_v3.Cache{},
			Classify: func(req *envoy_service_discovery_v3.DiscoveryRequest) string {
				return req.GetTypeUrl()
			},
			ClassifyDelta: func(dr *envoy_cache_v3.DeltaRequest) string {
				return dr.GetTypeUrl()
			},
		}
	)

	for _, resourceCache := range resources {
		if typeURL := resourceCache.TypeURL(); typeURL == envoy_resource_v3.EndpointType {
			mux.Caches[typeURL] = edsCache
		} else {
			mux.Caches[typeURL] = defaultCache
		}
	}

	sh := &SnapshotHandler{
		resources:    parseResources(resources),
		defaultCache: defaultCache,
		edsCache:     edsCache,
		mux:          mux,
		log:          log,
	}

	// Trigger an initial snapshot, based on any static values
	// present in the resource caches.
	sh.OnChange(nil)

	return sh
}

// GetCache returns the MuxCache, which multiplexes requests across
// underlying caches.
func (s *SnapshotHandler) GetCache() envoy_cache_v3.Cache {
	return s.mux
}

// Refresh is called when the EndpointsTranslator updates values
// in its cache. It updates the EDS cache.
func (s *SnapshotHandler) Refresh() {
	version := uuid.NewString()

	resources := map[envoy_resource_v3.Type][]envoy_types.Resource{
		envoy_resource_v3.EndpointType: asResources(s.resources[envoy_resource_v3.EndpointType].Contents()),
	}

	snapshot, err := envoy_cache_v3.NewSnapshot(version, resources)
	if err != nil {
		s.log.Errorf("failed to generate snapshot version %q: %s", version, err)
		return
	}

	if err := s.edsCache.SetSnapshot(context.Background(), contour_xds_v3.Hash.String(), snapshot); err != nil {
		s.log.Errorf("failed to store snapshot version %q: %s", version, err)
		return
	}
}

// OnChange is called when the DAG is rebuilt and a new snapshot is needed.
// It creates and caches a new go-control-plane Snapshot based on the
// contents of the Contour xDS resource caches.
func (s *SnapshotHandler) OnChange(*dag.DAG) {
	// Generate new snapshot version.
	version := uuid.NewString()

	// Convert caches to envoy xDS Resources.
	resources := map[envoy_resource_v3.Type][]envoy_types.Resource{}

	for resourceType, resourceCache := range s.resources {
		// Endpoints use their own cache.
		if resourceType == envoy_resource_v3.EndpointType {
			continue
		}

		resources[resourceType] = asResources(resourceCache.Contents())
	}

	snapshot, err := envoy_cache_v3.NewSnapshot(version, resources)
	if err != nil {
		s.log.Errorf("failed to generate snapshot version %q: %s", version, err)
		return
	}

	if err := s.defaultCache.SetSnapshot(context.Background(), contour_xds_v3.Hash.String(), snapshot); err != nil {
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
