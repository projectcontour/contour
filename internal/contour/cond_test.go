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

package contour

import "testing"

func TestCondRegisterBeforeNotifyShouldNotBroadcast(t *testing.T) {
	var c Cond
	ch := make(chan int, 1)
	c.Register(ch, 0)
	select {
	case <-ch:
		t.Fatal("ch was notified before broadcast")
	default:
	}
}

func TestCondRegisterAfterNotifyShouldBroadcast(t *testing.T) {
	var c Cond
	ch := make(chan int, 1)
	c.Notify()
	c.Register(ch, 0)
	select {
	case v := <-ch:
		if v != 1 {
			t.Fatal("ch was notified with the wrong sequence number", v)
		}
	default:
		t.Fatal("ch was not notified on registration")
	}
}

func TestCondRegisterAfterNotifyWithCorrectSequenceShouldNotBroadcast(t *testing.T) {
	var c Cond
	ch := make(chan int, 1)
	c.Notify()
	c.Register(ch, 0)
	seq := <-ch

	c.Register(ch, seq)
	select {
	case v := <-ch:
		t.Fatal("ch was notified immediately with seq", v)
	default:
	}
}

func TestCondRegisterWithHintShouldNotifyWithoutHint(t *testing.T) {
	var c Cond
	ch := make(chan int, 1)
	c.Register(ch, 1, "ingress_https")
	c.Notify()
	select {
	case v := <-ch:
		if v != 1 {
			t.Fatal("ch was notified with the wrong sequence number", v)
		}
	default:
		t.Fatal("ch was not notified")
	}
}

func TestCondRegisterWithHintShouldNotifyWithHint(t *testing.T) {
	var c Cond
	ch := make(chan int, 1)
	c.Register(ch, 1, "ingress_https")
	c.Notify("ingress_https")
	select {
	case v := <-ch:
		if v != 1 {
			t.Fatal("ch was notified with the wrong sequence number", v)
		}
	default:
		t.Fatal("ch was not notified")
	}
}

func TestCondRegisterWithHintShouldNotNotifyWithWrongHint(t *testing.T) {
	var c Cond
	ch := make(chan int, 1)
	c.Register(ch, 1, "ingress_https")
	c.Notify("banana")
	select {
	case v := <-ch:
		t.Fatal("ch was notified when it should not be", v)
	default:
	}
	c.Notify("ingress_https")
	select {
	case v := <-ch:
		if v != 2 {
			t.Fatal("ch was notified with the wrong sequence number", v)
		}
	default:
		t.Fatal("ch was not notified")
	}
}
