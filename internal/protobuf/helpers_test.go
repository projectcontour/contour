package protobuf

import (
	"testing"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/assert"
)

func TestU32Nil(t *testing.T) {
	assert.Equal(t, (*wrappers.UInt32Value)(nil), UInt32OrNil(0))
	assert.Equal(t, UInt32(1), UInt32OrNil(1))
}

func TestU32Default(t *testing.T) {
	assert.Equal(t, UInt32(99), UInt32OrDefault(0, 99))
	assert.Equal(t, UInt32(1), UInt32OrDefault(1, 99))
}
