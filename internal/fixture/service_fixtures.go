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

package fixture

import (
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var ServiceRootsKuard = &core_v1.Service{
	ObjectMeta: ObjectMeta("roots/kuard"),
	Spec: core_v1.ServiceSpec{
		Ports: []core_v1.ServicePort{{
			Name:       "http",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		}},
	},
}

var ServiceRootsHome = &core_v1.Service{
	ObjectMeta: ObjectMeta("roots/home"),
	Spec: core_v1.ServiceSpec{
		Ports: []core_v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     8080,
		}},
	},
}

var ServiceRootsFoo1 = &core_v1.Service{
	ObjectMeta: ObjectMeta("roots/foo1"),
	Spec: core_v1.ServiceSpec{
		Ports: []core_v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     8080,
		}},
	},
}

var ServiceRootsFoo2 = &core_v1.Service{
	ObjectMeta: ObjectMeta("roots/foo2"),
	Spec: core_v1.ServiceSpec{
		Ports: []core_v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     8080,
		}},
	},
}

var ServiceRootsFoo3InvalidPort = &core_v1.Service{
	ObjectMeta: ObjectMeta("roots/foo3"),
	Spec: core_v1.ServiceSpec{
		Ports: []core_v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     12345678,
		}},
	},
}

var ServiceMarketingGreen = &core_v1.Service{
	ObjectMeta: ObjectMeta("marketing/green"),
	Spec: core_v1.ServiceSpec{
		Ports: []core_v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     80,
		}},
	},
}

var ServiceRootsNginx = &core_v1.Service{
	ObjectMeta: ObjectMeta("roots/nginx"),
	Spec: core_v1.ServiceSpec{
		Ports: []core_v1.ServicePort{{
			Protocol: "TCP",
			Port:     80,
		}},
	},
}

var ServiceTeamAKuard = &core_v1.Service{
	ObjectMeta: ObjectMeta("teama/kuard"),
	Spec: core_v1.ServiceSpec{
		Ports: []core_v1.ServicePort{{
			Name:       "http",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		}},
	},
}

var ServiceTeamBKuard = &core_v1.Service{
	ObjectMeta: ObjectMeta("teamb/kuard"),
	Spec: core_v1.ServiceSpec{
		Ports: []core_v1.ServicePort{{
			Name:       "http",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		}},
	},
}
