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

package envoy

import (
	"log"

	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
)

type HTTPVersionType = http.HttpConnectionManager_CodecType

const (
	HTTPVersionAuto HTTPVersionType = http.HttpConnectionManager_AUTO
	HTTPVersion1    HTTPVersionType = http.HttpConnectionManager_HTTP1
	HTTPVersion2    HTTPVersionType = http.HttpConnectionManager_HTTP2
	HTTPVersion3    HTTPVersionType = http.HttpConnectionManager_HTTP3
)

// ProtoNamesForVersions returns the slice of ALPN protocol names for the give HTTP versions.
func ProtoNamesForVersions(versions ...HTTPVersionType) []string {
	protocols := map[HTTPVersionType]string{
		HTTPVersion1: "http/1.1",
		HTTPVersion2: "h2",
		HTTPVersion3: "",
	}
	defaultVersions := []string{"h2", "http/1.1"}
	wantedVersions := map[HTTPVersionType]struct{}{}

	if versions == nil {
		return defaultVersions
	}

	for _, v := range versions {
		wantedVersions[v] = struct{}{}
	}

	var alpn []string

	// Check for versions in preference order.
	for _, v := range []HTTPVersionType{HTTPVersionAuto, HTTPVersion2, HTTPVersion1} {
		if _, ok := wantedVersions[v]; ok {
			if v == HTTPVersionAuto {
				return defaultVersions
			}

			log.Printf("wanted %d -> %s", v, protocols[v])
			alpn = append(alpn, protocols[v])
		}
	}

	return alpn
}

// CodecForVersions determines a single Envoy HTTP codec constant
// that support all the given HTTP protocol versions.
func CodecForVersions(versions ...HTTPVersionType) HTTPVersionType {
	switch len(versions) {
	case 1:
		return versions[0]
	case 0:
		// Default is to autodetect.
		return HTTPVersionAuto
	default:
		// If more than one version is allowed, autodetect and let ALPN sort it out.
		return HTTPVersionAuto
	}
}
