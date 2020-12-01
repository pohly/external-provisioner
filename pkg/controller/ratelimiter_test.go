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

const factorMaxDelay = 10

func TestRateLimiter(t *testing.T) {
	maxDelay := factorMaxDelay * time.Second
	rd := newItemExponentialFailureRateLimiterWithJitter(time.Second, maxDelay, 0.05)

	// Drive up the scale factor by simulating 100% failure rate.
	for i := 0; i < 100; i++ {
		backoff := rd.When(1)
		if backoff > maxDelay {
			t.Errorf("expected value < %s, got %s", maxDelay, backoff)
		}
		rd.Forget(1)
		rd.Success(false)
		factor := rd.ScaleFactor()
		t.Logf("increase i=%d, scale factor %f", i, factor)
	}
	factor := rd.ScaleFactor()
	if factor < 0.99*factorMaxDelay {
		t.Errorf("expected scale factor >= 0.99 * %d, got %f", factorMaxDelay, factor)
	}

	// Reduce it back to 1.0 with zero failure rate.
	for i := 0; i < 100; i++ {
		backoff := rd.When(1)
		if backoff > maxDelay {
			t.Errorf("expected value < %s, got %s", maxDelay, backoff)
		}
		rd.Forget(1)
		rd.Success(true)
		factor := rd.ScaleFactor()
		t.Logf("decrease i=%d, scale factor %f", i, factor)
	}
	factor = rd.ScaleFactor()
	if factor > 1.1 {
		t.Errorf("expected scale factor <= 1.1, got %f", factor)
	}
}
