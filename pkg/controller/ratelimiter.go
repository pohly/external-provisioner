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
	"math"
	"math/rand"
	"sync"
	"time"
)

type rateLimiterWithJitter struct {
	exp       int
	baseDelay time.Duration
	maxDelay  time.Duration
	rd        *rand.Rand
	mutex     sync.Mutex
}

func (r *rateLimiterWithJitter) When() time.Duration {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// The delay is capped such that 'calculated' value never overflows.
	delay := float64(r.baseDelay.Nanoseconds()) * math.Pow(2, float64(r.exp))
	if delay > math.MaxInt64 ||
		int64(delay) > r.maxDelay.Nanoseconds() {
		return r.maxDelay
	}

	percentage := r.rd.Float64()
	jitter := float64(r.baseDelay.Nanoseconds()) * percentage
	backoff := delay - jitter
	if backoff <= 0 {
		return 0
	}
	return time.Duration(int64(backoff))
}

func (r *rateLimiterWithJitter) Failure() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Busy, slow down.
	r.exp++
}

func (r *rateLimiterWithJitter) Success() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Speed up gradually by reducing the delay exponent.
	if r.exp > 0 {
		r.exp--
	}
}

func (r *rateLimiterWithJitter) Exp() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.exp
}

func newItemExponentialFailureRateLimiterWithJitter(baseDelay time.Duration, maxDelay time.Duration) *rateLimiterWithJitter {
	return &rateLimiterWithJitter{
		baseDelay: baseDelay,
		maxDelay:  maxDelay,
		rd:        rand.New(rand.NewSource(time.Now().UTC().UnixNano())),
	}
}
