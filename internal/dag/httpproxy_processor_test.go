package dag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
)

func TestGetNamespacedName(t *testing.T) {
	testCases := []struct {
		name                   string
		input                  string
		expectedNamespacedName types.NamespacedName
		expectError            bool
	}{
		{
			name:                   "valid namespaced name",
			input:                  "foo/bar",
			expectedNamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"},
			expectError:            false,
		},
		{
			name:                   "invalid namespaced name",
			input:                  "foo",
			expectedNamespacedName: types.NamespacedName{},
			expectError:            true,
		},
		{
			name:                   "invalid namespaced name",
			input:                  "foo/bar/baz",
			expectedNamespacedName: types.NamespacedName{},
			expectError:            true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)
			actual, err := getNamespacedName(tc.input)
			a.Equal(tc.expectedNamespacedName, actual)
			a.Equal(tc.expectError, err != nil)
		})
	}
}
