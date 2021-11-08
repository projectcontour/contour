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

//go:generate mockery --case=snake --name=Manager --srcpkg=sigs.k8s.io/controller-runtime/pkg/manager

func initMockManager(t *testing.T) *mocks.Manager {
	t.Helper()

	mockManager := &mocks.Manager{}

	// TODO: see if there is a way we can automatically ignore these.
	mockManager.On("GetClient").Return(nil).Maybe()
	mockManager.On("GetLogger").Return(logr_testing.TestLogger{T: t}).Maybe()
	mockManager.On("SetFields", mock.Anything).Return(nil).Maybe()
	mockManager.On("Elected").Return(nil).Maybe()

	return mockManager
}

func TestRegisterGatewayController(t *testing.T) {
	mockManager := initMockManager(t)

	// Assert controller is added to manager and is a leader election runnable.
	mockManager.On("Add", mock.MatchedBy(func(manager.LeaderElectionRunnable) bool {
		return true
	})).Return(nil).Once()

	err := controller.RegisterGatewayController(fixture.NewTestLogger(t), mockManager, nil, nil, "some-controller")
	require.NoError(t, err)

	mockManager.AssertExpectations(t)
}

func TestRegisterGatewayClassController(t *testing.T) {
	mockManager := initMockManager(t)

	// Assert controller is added to manager and is a leader election runnable.
	mockManager.On("Add", mock.MatchedBy(func(manager.LeaderElectionRunnable) bool {
		return true
	})).Return(nil).Once()

	err := controller.RegisterGatewayClassController(fixture.NewTestLogger(t), mockManager, nil, nil, "some-gateway")
	require.NoError(t, err)

	mockManager.AssertExpectations(t)
}

func TestRegisterHTTPRouteController(t *testing.T) {
	mockManager := initMockManager(t)

	// Assert controller is added to manager and is a leader election runnable.
	mockManager.On("Add", mock.MatchedBy(func(manager.LeaderElectionRunnable) bool {
		return true
	})).Return(nil).Once()

	err := controller.RegisterHTTPRouteController(fixture.NewTestLogger(t), mockManager, nil)
	require.NoError(t, err)

	mockManager.AssertExpectations(t)
}

func TestRegisterTCPRouteController(t *testing.T) {
	mockManager := initMockManager(t)

	// Assert controller is added to manager and is a leader election runnable.
	mockManager.On("Add", mock.MatchedBy(func(manager.LeaderElectionRunnable) bool {
		return true
	})).Return(nil).Once()

	err := controller.RegisterTCPRouteController(fixture.NewTestLogger(t), mockManager, nil, "some-controller")
	require.NoError(t, err)

	mockManager.AssertExpectations(t)
}

func TestRegisterTLSRouteController(t *testing.T) {
	mockManager := initMockManager(t)

	// Assert controller is added to manager and is a leader election runnable.
	mockManager.On("Add", mock.MatchedBy(func(manager.LeaderElectionRunnable) bool {
		return true
	})).Return(nil).Once()

	err := controller.RegisterTLSRouteController(fixture.NewTestLogger(t), mockManager, nil)
	require.NoError(t, err)

	mockManager.AssertExpectations(t)
}

func TestRegisterUDPRouteController(t *testing.T) {
	mockManager := initMockManager(t)

	// Assert controller is added to manager and is a leader election runnable.
	mockManager.On("Add", mock.MatchedBy(func(manager.LeaderElectionRunnable) bool {
		return true
	})).Return(nil).Once()

	err := controller.RegisterUDPRouteController(fixture.NewTestLogger(t), mockManager, nil, "some-controller")
	require.NoError(t, err)

	mockManager.AssertExpectations(t)
}
