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

package xdscache

import (
	"math"
	"reflect"
	"strconv"

	envoy_xds "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v2"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/xds"
	"github.com/sirupsen/logrus"
)

// SnapshotHandler implements the xDS snapshot cache
// by responding to the OnChange() event causing a new
// snapshot to be created.
type SnapshotHandler struct {

	// resources holds the cache of xDS contents.
	resources map[envoy_xds.ResponseType]ResourceCache

	// snapshotCache is a snapshot-based cache that maintains a single versioned
	// snapshot of responses for xDS resources that Contour manages.
	snapshotCache cache.SnapshotCache

	// snapshotVersion holds the current version of the snapshot.
	snapshotVersion int64

	logrus.FieldLogger
}

// NewSnapshotHandler returns an instance of SnapshotHandler.
func NewSnapshotHandler(c cache.SnapshotCache, resources []ResourceCache, logger logrus.FieldLogger) *SnapshotHandler {

	sh := &SnapshotHandler{
		snapshotCache: c,
		resources:     parseResources(resources),
		FieldLogger:   logger,
	}

	return sh
}

// Refresh is called when the EndpointsTranslator updates values
// in its cache.
func (s *SnapshotHandler) Refresh() {
	s.generateNewSnapshot()
}

// OnChange is called when the DAG is rebuilt and a new snapshot is needed.
func (s *SnapshotHandler) OnChange(root *dag.DAG) {
	s.generateNewSnapshot()
}

// generateNewSnapshot creates a new snapshot against
// the Contour XDS caches.
func (s *SnapshotHandler) generateNewSnapshot() {
	// Generate new snapshot version.
	snapshotVersion := s.newSnapshotVersion()

	// Create an snapshot with all xDS resources.
	snapshot := cache.NewSnapshot(snapshotVersion,
		asResources(s.resources[envoy_xds.Endpoint].Contents()),
		asResources(s.resources[envoy_xds.Cluster].Contents()),
		asResources(s.resources[envoy_xds.Route].Contents()),
		asResources(s.resources[envoy_xds.Listener].Contents()),
		nil)

	// Update the Secrets xDS resource manually until a new version of go-control-plane is released.
	// ref: https://github.com/envoyproxy/go-control-plane/pull/314
	snapshot.Resources[envoy_xds.Secret] = cache.NewResources(snapshotVersion, asResources(s.resources[envoy_xds.Secret].Contents()))

	if err := s.snapshotCache.SetSnapshot(xds.DefaultHash.String(), snapshot); err != nil {
		s.Errorf("OnChange: Error setting snapshot: %q", err)
	}
}

// newSnapshotVersion increments the current snapshotVersion
// and returns as a string.
func (s *SnapshotHandler) newSnapshotVersion() string {

	// Reset the snapshotVersion if it ever hits max size.
	if s.snapshotVersion == math.MaxInt64 {
		s.snapshotVersion = 0
	}

	// Increment the snapshot version & return as string.
	s.snapshotVersion++
	return strconv.FormatInt(s.snapshotVersion, 10)
}

// asResources casts the given slice of values (that implement the envoy_xds.Resource
// interface) to a slice of envoy_xds.Resource. If the length of the slice is 0, it
// returns nil.
func asResources(messages interface{}) []envoy_xds.Resource {
	v := reflect.ValueOf(messages)
	if v.Len() == 0 {
		return nil
	}

	protos := make([]envoy_xds.Resource, v.Len())

	for i := range protos {
		protos[i] = v.Index(i).Interface().(envoy_xds.Resource)
	}

	return protos
}

// parseResources converts an []ResourceCache to a map[envoy_xds.ResponseType]ResourceCache
// for faster indexing when creating new snapshots.
func parseResources(resources []ResourceCache) map[envoy_xds.ResponseType]ResourceCache {

	resourceMap := make(map[envoy_xds.ResponseType]ResourceCache, len(resources))

	for _, r := range resources {
		switch r.TypeURL() {
		case resource.ClusterType:
			resourceMap[envoy_xds.Cluster] = r
		case resource.RouteType:
			resourceMap[envoy_xds.Route] = r
		case resource.ListenerType:
			resourceMap[envoy_xds.Listener] = r
		case resource.SecretType:
			resourceMap[envoy_xds.Secret] = r
		case resource.EndpointType:
			resourceMap[envoy_xds.Endpoint] = r
		}
	}
	return resourceMap
}
