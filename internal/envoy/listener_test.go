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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCodecForVersions(t *testing.T) {
	assert.Equal(t, CodecForVersions(HTTPVersionAuto), HTTPVersionAuto)
	assert.Equal(t, CodecForVersions(HTTPVersion1, HTTPVersion2), HTTPVersionAuto)
	assert.Equal(t, CodecForVersions(HTTPVersion1), HTTPVersion1)
	assert.Equal(t, CodecForVersions(HTTPVersion2), HTTPVersion2)
}

func TestProtoNamesForVersions(t *testing.T) {
	assert.Equal(t, ProtoNamesForVersions(), []string{"h2", "http/1.1"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersionAuto), []string{"h2", "http/1.1"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion1), []string{"http/1.1"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion2), []string{"h2"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion3), []string(nil))
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion1, HTTPVersion2), []string{"h2", "http/1.1"})
}
