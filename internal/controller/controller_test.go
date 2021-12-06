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

package controller_test

import (
	"testing"

	logr_testing "github.com/go-logr/logr/testing"
	"github.com/projectcontour/contour/internal/controller"
	"github.com/projectcontour/contour/internal/controller/mocks"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

//go:generate go run github.com/vektra/mockery/v2 --case=snake --name=Manager --srcpkg=sigs.k8s.io/controller-runtime/pkg/manager

func TestRegisterControllers(t *testing.T) {
	tests := map[string]func(*mocks.Manager) error{
		"gateway controller": func(mockManager *mocks.Manager) error {
			_, err := controller.RegisterGatewayController(fixture.NewTestLogger(t), mockManager, nil, nil, "some-controller")
			return err
		},
		"gatewayclass controller": func(mockManager *mocks.Manager) error {
			_, err := controller.RegisterGatewayClassController(fixture.NewTestLogger(t), mockManager, nil, nil, "some-gateway")
			return err
		},
		"httproute controller": func(mockManager *mocks.Manager) error {
			return controller.RegisterHTTPRouteController(fixture.NewTestLogger(t), mockManager, nil)
		},
		"tlsroute controller": func(mockManager *mocks.Manager) error {
			return controller.RegisterTLSRouteController(fixture.NewTestLogger(t), mockManager, nil)
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockManager := &mocks.Manager{}

			// TODO: see if there is a way we can automatically ignore these.
			mockManager.On("GetClient").Return(nil).Maybe()
			mockManager.On("GetLogger").Return(logr_testing.TestLogger{T: t}).Maybe()
			mockManager.On("SetFields", mock.Anything).Return(nil).Maybe()
			mockManager.On("Elected").Return(nil).Maybe()

			mockManager.On("Add", mock.MatchedBy(func(r manager.LeaderElectionRunnable) bool {
				return r.NeedLeaderElection() == false
			})).Return(nil).Once()

			require.NoError(t, test(mockManager))

			require.True(t, mockManager.AssertExpectations(t))
		})
	}
}
