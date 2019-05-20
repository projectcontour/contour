// Copyright © 2019 Heptio
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

import _cache "github.com/envoyproxy/go-control-plane/pkg/cache"

// TODO(dfc) these are repeated here because this package declares a type
// called cache. Rather than renaming the import across five other files
// we do so here to make things simpler. Ideally this wouldn't be necessary.
const (
	endpointType = _cache.EndpointType
	clusterType  = _cache.ClusterType
	routeType    = _cache.RouteType
	listenerType = _cache.ListenerType
	secretType   = _cache.SecretType
)
