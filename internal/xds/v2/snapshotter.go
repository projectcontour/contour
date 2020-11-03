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

package v2

import (
	envoy_types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoy_cache_v2 "github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	envoy_log "github.com/envoyproxy/go-control-plane/pkg/log"
	"github.com/projectcontour/contour/internal/xds"
	"github.com/projectcontour/contour/internal/xdscache"
)

var Hash = xds.ConstantHashV2{}

// Snapshotter is a v2 Snapshot cache that implements the xds.Snapshotter interface.
type Snapshotter interface {
	xdscache.Snapshotter
	envoy_cache_v2.SnapshotCache
}

type snapshotter struct {
	envoy_cache_v2.SnapshotCache
}

func (s *snapshotter) Generate(version string, resources map[envoy_types.ResponseType][]envoy_types.Resource) error {
	// Create a snapshot with all xDS resources.
	snapshot := envoy_cache_v2.NewSnapshot(
		version,
		resources[envoy_types.Endpoint],
		resources[envoy_types.Cluster],
		resources[envoy_types.Route],
		resources[envoy_types.Listener],
		nil,
		resources[envoy_types.Secret],
	)

	return s.SetSnapshot(Hash.String(), snapshot)
}

func NewSnapshotCache(ads bool, logger envoy_log.Logger) Snapshotter {
	return &snapshotter{
		SnapshotCache: envoy_cache_v2.NewSnapshotCache(ads, &Hash, logger),
	}
}
