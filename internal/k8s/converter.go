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

package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type Converter interface {
	FromUnstructured(obj interface{}) (interface{}, error)
	ToUnstructured(obj interface{}) (*unstructured.Unstructured, error)
}

// UnstructuredConverter handles conversions between unstructured.Unstructured and Contour types
type UnstructuredConverter struct {
	// scheme holds an initializer for converting Unstructured to a type
	scheme *runtime.Scheme
}

// NewUnstructuredConverter returns a new UnstructuredConverter initialized
func NewUnstructuredConverter() (*UnstructuredConverter, error) {
	s, err := NewContourScheme()
	if err != nil {
		return nil, err
	}

	return &UnstructuredConverter{scheme: s}, nil
}

// FromUnstructured converts an unstructured.Unstructured to typed struct. If obj
// is not an unstructured.Unstructured it is returned without further processing.
func (c *UnstructuredConverter) FromUnstructured(obj interface{}) (interface{}, error) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return obj, nil
	}

	newObj, err := c.scheme.New(u.GetObjectKind().GroupVersionKind())
	if err != nil {
		return nil, err
	}

	return newObj, c.scheme.Convert(obj, newObj, nil)
}

// ToUnstructured converts the supplied object to Unstructured, provided it's one of the types
// registered in the UnstructuredConverter's Scheme.
func (c *UnstructuredConverter) ToUnstructured(obj interface{}) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}

	if err := c.scheme.Convert(obj, u, context.TODO()); err != nil {
		return nil, err
	}

	return u, nil
}
