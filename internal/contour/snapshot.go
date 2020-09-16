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

package contour

import (
	"math"
	"reflect"
	"strconv"

	envoy_xds "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/xds"
	"github.com/sirupsen/logrus"
)

// SnapshotHandler implements the xDS snapshot cache
// by responding to the OnChange() event causing a new
// snapshot to be created.
type SnapshotHandler struct {

	// resources holds the cache of xDS contents.
	resources map[envoy_xds.ResponseType]ResourceSnapshot

	// snapshotCache is a snapshot-based cache that maintains a single versioned
	// snapshot of responses for xDS resources that Contour manages.
	snapshotCache cache.SnapshotCache

	logrus.FieldLogger
}

type ResourceSnapshot struct {
	ResourceCache

	// snapshotVersion holds the current version of the snapshot.
	snapshotVersion int64
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

	snapshot := cache.Snapshot{}
	for key, r := range s.resources {

		// Check if the contents of the snapshot cache have changed.
		currentSnap, err := s.snapshotCache.GetSnapshot("contour")
		if err != nil {
			s.Debugf("OnChange: Error getting current snapshot for resource %q: %q", r.TypeURL(), err)
		}

		// If caches are different, set new snapshot.
		if !reflect.DeepEqual(currentSnap.Resources[key].Items, cache.IndexResourcesByName(asResources(r.Contents()))) {

			// Create a new cache resource to compare against current snapshot.
			snapshot.Resources[key] = cache.NewResources(r.nextSnapshotVersion(), asResources(r.Contents()))
		} else {

			// If caches match, set the old resources to make the snapshot complete.
			snapshot.Resources[key] = cache.Resources{
				Version: currentSnap.GetVersion(r.TypeURL()),
				Items:   currentSnap.Resources[key].Items,
			}
		}
	}

	if err := s.snapshotCache.SetSnapshot(xds.DefaultHash.String(), snapshot); err != nil {
		s.Errorf("OnChange: Error setting snapshot: %q", err)
	}
}

// nextSnapshotVersion increments the current snapshotVersion
// and returns as a string.
func (r *ResourceSnapshot) nextSnapshotVersion() string {

	// Reset the snapshotVersion if it ever hits max size.
	if r.snapshotVersion == math.MaxInt64 {
		r.snapshotVersion = 0
	}

	// Increment the snapshot version & return as string.
	r.snapshotVersion++
	return strconv.FormatInt(r.snapshotVersion, 10)
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

// parseResources converts an []ResourceCache to a map[envoy_xds.ResponseType]ResourceSnapshot
// for faster indexing when creating new snapshots.
func parseResources(resources []ResourceCache) map[envoy_xds.ResponseType]ResourceSnapshot {

	resourceMap := make(map[envoy_xds.ResponseType]ResourceSnapshot, len(resources))

	for _, r := range resources {
		resourceMap[cache.GetResponseType(r.TypeURL())] = ResourceSnapshot{
			ResourceCache:   r,
			snapshotVersion: 0,
		}
	}
	return resourceMap
}
