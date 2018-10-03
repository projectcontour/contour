// Copyright © 2018 Heptio
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
	"fmt"
	"testing"

	ingressroutev1beta1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/apis/generated/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

func TestSetStatus(t *testing.T) {
	tests := map[string]struct {
		msg           string
		desc          string
		existing      *ingressroutev1beta1.IngressRoute
		expectedPatch string
		expectedVerbs []string
	}{
		"simple update": {
			msg:  "valid",
			desc: "this is a valid IR",
			existing: &ingressroutev1beta1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Status: ingressroutev1beta1.Status{
					CurrentStatus: "",
					Description:   "",
				},
			},
			expectedPatch: `{"status":{"currentStatus":"valid","description":"this is a valid IR"}}`,
			expectedVerbs: []string{"patch"},
		},
		"no update": {
			msg:  "valid",
			desc: "this is a valid IR",
			existing: &ingressroutev1beta1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Status: ingressroutev1beta1.Status{
					CurrentStatus: "valid",
					Description:   "this is a valid IR",
				},
			},
			expectedPatch: ``,
			expectedVerbs: []string{},
		},
		"replace existing status": {
			msg:  "valid",
			desc: "this is a valid IR",
			existing: &ingressroutev1beta1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Status: ingressroutev1beta1.Status{
					CurrentStatus: "invalid",
					Description:   "boo hiss",
				},
			},
			expectedPatch: `{"status":{"currentStatus":"valid","description":"this is a valid IR"}}`,
			expectedVerbs: []string{"patch"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var gotPatchBytes []byte
			client := fake.NewSimpleClientset(tc.existing)
			client.PrependReactor("patch", "ingressroutes", func(action k8stesting.Action) (bool, runtime.Object, error) {
				switch patchAction := action.(type) {
				default:
					return true, nil, fmt.Errorf("got unexpected action of type: %T", action)
				case k8stesting.PatchActionImpl:
					gotPatchBytes = patchAction.GetPatch()
					return true, tc.existing, nil
				}
			})
			irs := IngressRouteStatus{
				Client: client,
			}
			if err := irs.SetStatus(tc.msg, tc.desc, tc.existing); err != nil {
				t.Fatal(err)
			}

			if len(client.Actions()) != len(tc.expectedVerbs) {
				t.Fatalf("Expected verbs mismatch: want: %d, got: %d", len(tc.expectedVerbs), len(client.Actions()))
			}

			if tc.expectedPatch != string(gotPatchBytes) {
				t.Fatalf("expected patch: %s, got: %s", tc.expectedPatch, string(gotPatchBytes))
			}
		})
	}
}
