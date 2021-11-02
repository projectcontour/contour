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

package contourconfiguration

import (
	"fmt"

	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/timeout"
)

type Timeouts struct {
	Request                       timeout.Setting
	ConnectionIdle                timeout.Setting
	StreamIdle                    timeout.Setting
	MaxConnectionDuration         timeout.Setting
	DelayedClose                  timeout.Setting
	ConnectionShutdownGracePeriod timeout.Setting
}

func ParseTimeoutPolicy(timeoutParameters *contour_api_v1alpha1.TimeoutParameters) (Timeouts, error) {
	var (
		err      error
		timeouts Timeouts
	)

	if timeoutParameters != nil {
		if timeoutParameters.RequestTimeout != nil {
			timeouts.Request, err = timeout.Parse(*timeoutParameters.RequestTimeout)
			if err != nil {
				return Timeouts{}, fmt.Errorf("failed to parse request timeout: %s", err)
			}
		}
		if timeoutParameters.ConnectionIdleTimeout != nil {
			timeouts.ConnectionIdle, err = timeout.Parse(*timeoutParameters.ConnectionIdleTimeout)
			if err != nil {
				return Timeouts{}, fmt.Errorf("failed to parse connection idle timeout: %s", err)
			}
		}
		if timeoutParameters.StreamIdleTimeout != nil {
			timeouts.StreamIdle, err = timeout.Parse(*timeoutParameters.StreamIdleTimeout)
			if err != nil {
				return Timeouts{}, fmt.Errorf("failed to parse stream idle timeout: %s", err)
			}
		}
		if timeoutParameters.MaxConnectionDuration != nil {
			timeouts.MaxConnectionDuration, err = timeout.Parse(*timeoutParameters.MaxConnectionDuration)
			if err != nil {
				return Timeouts{}, fmt.Errorf("failed to parse max connection duration: %s", err)
			}
		}
		if timeoutParameters.DelayedCloseTimeout != nil {
			timeouts.DelayedClose, err = timeout.Parse(*timeoutParameters.DelayedCloseTimeout)
			if err != nil {
				return Timeouts{}, fmt.Errorf("failed to parse delayed close timeout: %s", err)
			}
		}
		if timeoutParameters.ConnectionShutdownGracePeriod != nil {
			timeouts.ConnectionShutdownGracePeriod, err = timeout.Parse(*timeoutParameters.ConnectionShutdownGracePeriod)
			if err != nil {
				return Timeouts{}, fmt.Errorf("failed to parse connection shutdown grace period: %s", err)
			}
		}
	}
	return timeouts, nil
}
