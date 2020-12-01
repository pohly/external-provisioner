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
	"math/rand"
	"sync"
	"time"

	"k8s.io/client-go/util/workqueue"
)

type rateLimiterWithJitter struct {
	workqueue.RateLimiter
	baseDelay   time.Duration
	maxDelay    time.Duration
	alpha       float64
	scaleFactor float64
	failureRate float64
	rd          *rand.Rand
	mutex       sync.Mutex
}

func (r *rateLimiterWithJitter) When(item interface{}) time.Duration {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	delay := int64(float64(r.RateLimiter.When(item).Nanoseconds()) * r.scaleFactor)
	if delay > r.maxDelay.Nanoseconds() {
		delay = r.maxDelay.Nanoseconds()
	}
	percentage := r.rd.Float64()
	jitter := int64(float64(r.baseDelay.Nanoseconds()) * percentage)
	if jitter > delay {
		return 0
	}
	return time.Duration(delay - jitter)
}

func (r *rateLimiterWithJitter) ScaleFactor() float64 {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.scaleFactor
}

func (r *rateLimiterWithJitter) Success(success bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// For 100% failure rate, we aim for multiplying by maxDelay/baseDelay,
	// so the scaled value will be closer to maxDelay.
	// For 0% failure rate, we aim for multiplying by 1 (the starting value).
	// Inbetween we increase linearly:
	//
	// factor := rate * (maxDelay/baseDelay - 1.0) + 1.0
	//
	// The failure rate is calculated as exponential moving average
	// (https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average).
	newSample := 0.0
	if !success {
		newSample = 1.0
	}
	r.failureRate = r.alpha*newSample + (1.0-r.alpha)*r.failureRate
	r.scaleFactor = r.failureRate*(float64(r.maxDelay.Nanoseconds())/float64(r.baseDelay.Nanoseconds())-1.0) + 1.0
}

func newItemExponentialFailureRateLimiterWithJitter(baseDelay time.Duration, maxDelay time.Duration, alpha float64) *rateLimiterWithJitter {
	return &rateLimiterWithJitter{
		RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(baseDelay, maxDelay),
		baseDelay:   baseDelay,
		maxDelay:    maxDelay,
		failureRate: 0,
		scaleFactor: 1.0,
		alpha:       alpha,
		rd:          rand.New(rand.NewSource(time.Now().UTC().UnixNano())),
	}
}
