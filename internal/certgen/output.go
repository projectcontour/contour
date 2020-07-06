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

package certgen

import (
	"fmt"
	"os"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)

// writeSecret writes out a given Secret to a file.
func writeSecret(f *os.File, secret *corev1.Secret) error {
	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme)
	return s.Encode(secret, f)
}

func createFile(filepath string, force bool) (*os.File, error) {

	err := os.MkdirAll(path.Dir(filepath), 0755)
	if err != nil {
		return nil, fmt.Errorf("unable to create %s: %s", path.Dir(filepath), err)
	}

	flags := os.O_RDWR | os.O_CREATE | os.O_TRUNC
	if !force {
		flags = flags | os.O_EXCL
	}

	f, err := os.OpenFile(filepath, flags, 0666)
	if err != nil {
		// File exists, and we don't want to create it.
		return nil, fmt.Errorf("can't create file %s: %s", filepath, err)
	}
	fmt.Printf("%s created\n", filepath)
	return f, nil
}

// checkFile is a helper to tidy up a file in the event something went wrong
// when writing it.
func checkFile(filename string, err error) error {
	if err != nil {
		// Clean up our possibly partially written file
		removeErr := os.Remove(filename)
		if removeErr != nil {
			return fmt.Errorf("couldn't write out %s and failed to clean up, %s", filename, removeErr)
		}
		return err
	}
	return nil
}
