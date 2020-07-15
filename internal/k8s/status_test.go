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
	"errors"
	"fmt"
	"testing"

	"github.com/projectcontour/contour/internal/assert"

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetHTTPProxyStatus(t *testing.T) {
	type testcase struct {
		msg      string
		desc     string
		existing *projectcontour.HTTPProxy
		expected *projectcontour.HTTPProxy
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Helper()

			suc := &StatusUpdateCacher{}
			proxysw := StatusWriter{
				Updater: suc,
			}

			suc.AddObject(tc.existing.Name, tc.existing.Namespace, projcontour.HTTPProxyGVR, tc.existing)

			if err := proxysw.SetStatus(tc.msg, tc.desc, tc.existing); err != nil {
				t.Fatal(fmt.Errorf("unable to set proxy status: %s", err))
			}

			toProxy := suc.GetObject(tc.existing.Name, tc.existing.Namespace, projcontour.HTTPProxyGVR)

			if toProxy == nil && tc.expected == nil {
				return
			}

			assert.Equal(t, toProxy, tc.expected)

			if toProxy == nil && tc.expected != nil {
				t.Fatalf("Did not get expected update, %#v", tc.expected)
			}
		})
	}

	run(t, "simple update", testcase{
		msg:  "valid",
		desc: "this is a valid HTTPProxy",
		existing: &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Status: projcontour.HTTPProxyStatus{
				CurrentStatus: "",
				Description:   "",
			},
		},
		expected: &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Status: projcontour.HTTPProxyStatus{
				CurrentStatus: "valid",
				Description:   "this is a valid HTTPProxy",
			},
		},
	})

	run(t, "no update", testcase{
		msg:  "valid",
		desc: "this is a valid HTTPProxy",
		existing: &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Status: projcontour.HTTPProxyStatus{
				CurrentStatus: "valid",
				Description:   "this is a valid HTTPProxy",
			},
		},
		expected: &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Status: projcontour.HTTPProxyStatus{
				CurrentStatus: "valid",
				Description:   "this is a valid HTTPProxy",
			},
		},
	})

	run(t, "replace existing status", testcase{
		msg:  "valid",
		desc: "this is a valid HTTPProxy",
		existing: &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Status: projcontour.HTTPProxyStatus{
				CurrentStatus: "invalid",
				Description:   "boo hiss",
			},
		},
		expected: &projcontour.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Status: projcontour.HTTPProxyStatus{
				CurrentStatus: "valid",
				Description:   "this is a valid HTTPProxy",
			},
		},
	})
}

func TestGetStatus(t *testing.T) {
	type testcase struct {
		input          interface{}
		expectedStatus *projcontour.HTTPProxyStatus
		expectedError  error
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Helper()

			proxysw := StatusWriter{}

			status, err := proxysw.GetStatus(tc.input)

			assert.Equal(t, tc.expectedStatus, status)
			assert.Equal(t, tc.expectedError, err)
		})
	}

	run(t, "not implemented", testcase{
		input:          nil,
		expectedStatus: nil,
		expectedError:  errors.New("not implemented"),
	})
}
