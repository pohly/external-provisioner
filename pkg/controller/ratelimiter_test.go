/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rd := newItemExponentialFailureRateLimiterWithJitter(time.Second, 4*time.Second)
	backoff := rd.When()
	if backoff > time.Second {
		t.Errorf("expected value < 1s, got %s", backoff)
	}

	rd.Failure()
	exp := rd.Exp()
	if exp != 1 {
		t.Errorf("expected exp == 1, got %d", exp)
	}
	backoff = rd.When()
	if backoff < time.Second ||
		backoff > 2*time.Second {
		t.Errorf("expected value >= 1s, <= 2s, got %s", backoff)
	}

	rd.Failure()
	exp = rd.Exp()
	if exp != 2 {
		t.Errorf("expected exp == 2, got %d", exp)
	}
	backoff = rd.When()
	if backoff < 2*time.Second ||
		backoff > 4*time.Second {
		t.Errorf("expected value >= 2s, <= 4s, got %s", backoff)
	}

	rd.Failure()
	exp = rd.Exp()
	if exp != 3 {
		t.Errorf("expected exp == 3, got %d", exp)
	}
	backoff = rd.When()
	if backoff < 2*time.Second ||
		backoff > 4*time.Second {
		t.Errorf("expected value >= 2s, <= 4s, got %s", backoff)
	}

	rd.Success()
	exp = rd.Exp()
	if exp != 0 {
		t.Errorf("expected exp == 0, got %d", exp)
	}
	backoff = rd.When()
	if backoff > time.Second {
		t.Errorf("expected value <= 1s, got %s", backoff)
	}
}
