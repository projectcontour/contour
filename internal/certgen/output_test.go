// Copyright © 2020 VMware
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
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateFile(t *testing.T) {
	basedir, err := ioutil.TempDir("", "test-create-file")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(basedir)

	path := filepath.Join(basedir, "f")

	f, err := createFile(path, false) // no force
	if err != nil {
		t.Fatalf("createFile with a non existant path should succeed, got: %v", err)
	}
	mustClose(t, f)

	f, err = createFile(path, false) // no force
	if err == nil {
		t.Fatal("createFile on an existing file without force should fail")
		mustClose(t, f)
	}

	f, err = createFile(path, true) // force
	if err != nil {
		t.Fatalf("createFile on an existing file with force should succeed, got: %v", err)
	}
	mustClose(t, f)
}

func mustClose(t *testing.T, c io.Closer) {
	err := c.Close()
	if err != nil {
		t.Helper()
		t.Fatal(err)
	}
}
