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

package routegen

import (
	"bytes"
	"io"
	"os"

	"github.com/sirupsen/logrus"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

var (
	scheme       = runtime.NewScheme()
	codecFactory = serializer.NewCodecFactory(scheme)
	deserializer = codecFactory.UniversalDeserializer()
)

func init() {
	_ = contour_v1.AddToScheme(scheme)
	_ = contour_v1alpha1.AddToScheme(scheme)
	_ = core_v1.AddToScheme(scheme)
	_ = apps_v1.AddToScheme(scheme)
}

// ReadManifestFiles reads Kubernetes manifest files and returns the resources as runtime objects.
func ReadManifestFiles(manifests []string, logger *logrus.Logger) ([]runtime.Object, error) {
	var resources []runtime.Object

	for _, manifestPath := range manifests {
		res, err := readManifest(manifestPath, logger)
		if err != nil {
			return nil, err
		}
		resources = append(resources, res...)
	}

	return resources, nil
}

// readManifest reads and decodes all resources defined in a single manifest file.
func readManifest(filePath string, logger *logrus.Logger) ([]runtime.Object, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return decodeManifest(content, logger)
}

// decodeManifest decodes a byte slice of manifest data into Kubernetes objects.
func decodeManifest(data []byte, logger *logrus.Logger) ([]runtime.Object, error) {
	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(data), 1024)
	var resources []runtime.Object

	for {
		var rawObj runtime.RawExtension
		err := decoder.Decode(&rawObj)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		if len(rawObj.Raw) == 0 {
			continue // Skip empty documents
		}

		obj, decodeErr := decodeRuntimeObject(rawObj)
		if decodeErr != nil {
			if runtime.IsNotRegisteredError(decodeErr) {
				logger.Warnf("Skipping unregistered resource due to: %v", decodeErr)
				continue
			}
			return nil, decodeErr
		}

		resources = append(resources, obj)
	}

	return resources, nil
}

// decodeRuntimeObject decodes a raw extension into a runtime object using the deserializer.
func decodeRuntimeObject(raw runtime.RawExtension) (runtime.Object, error) {
	obj, _, err := deserializer.Decode(raw.Raw, nil, nil)
	return obj, err
}
