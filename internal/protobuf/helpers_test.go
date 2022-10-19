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

package protobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestU32Nil(t *testing.T) {
	assert.Equal(t, (*wrapperspb.UInt32Value)(nil), UInt32OrNil(0))
	assert.Equal(t, UInt32(1), UInt32OrNil(1))
}

func TestU32Default(t *testing.T) {
	assert.Equal(t, UInt32(99), UInt32OrDefault(0, 99))
	assert.Equal(t, UInt32(1), UInt32OrDefault(1, 99))
}
