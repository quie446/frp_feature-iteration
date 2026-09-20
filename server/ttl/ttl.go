// Copyright 2026 The frp Authors
//
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

// Package ttl implements the server-side lifetime ledger for temporary
// proxies. It is deliberately independent of Control: the registry only
// remembers proxy names and their absolute expiry instants, and asks the
// owner (server.Service) to tear an expired proxy down via OnExpire.
package ttl

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/utils/clock"

	"github.com/fatedier/frp/pkg/util/log"
)

// ExpiredError is returned when a client tries to register a proxy whose
// name is already recorded as expired. The message is safe to surface to
// frpc operators.
type ExpiredError struct {
	Name    string
	Expired time.Time
}

func (e *ExpiredError) Error() string {
	return fmt.Sprintf("proxy [%s] is expired at %s, re-registration is denied until the TTL grant is cleared",
		e.Name, e.Expired.Format(time.RFC3339))
}

type entry struct {
	name      string
	requested time.Duration
	expiresAt time.Time
	expired   bool
	timer     clock.Timer
}

// Info is a point-in-time view of a TTL record, used by the admin API.
type Info struct {
	Name          string
	RequestedTTL  time.Duration
	ExpiresAt     time.Time
	Remaining     time.Duration
	Expired       bool
	LastExpiredAt time.Time
}

// Registry keeps one TTL record per proxy name. Records survive client
// disconnects so that an expired temporary tunnel cannot be silently
// revived by reconnecting with the same proxy name.
type Registry struct {
	clk      clock.Clock
	onExpire func(name string)

	mu      sync.Mutex
	entries map[string]*entry
	closed  bool
}

func NewRegistry(onExpire func(name string)) *Registry {
	return newRegistryWithClock(clock.RealClock{}, onExpire)
}

// NewRegistryWithClockForTest builds a registry driven by an arbitrary
// clock. It is intended for tests.
func NewRegistryWithClockForTest(clk clock.Clock, onExpire func(name string)) *Registry {
	return newRegistryWithClock(clk, onExpire)
}

func newRegistryWithClock(clk clock.Clock, onExpire func(name string)) *Registry {
	return &Registry{
		clk:      clk,
		onExpire: onExpire,
		entries:  make(map[string]*entry),
	}
}

// Admit is called when a proxy registration is about to succeed.
//
//   - If the name has never carried a TTL, a new record is created.
//   - If the record is still alive (client reconnected before expiry), the
//     original deadline is kept: reconnecting never extends the lifetime.
//   - If the record is expired, registration is rejected.
//
// ttl <= 0 is rejected here as a defensive check even though validation
// already rejects such values on the client side.
func (r *Registry) Admit(name string, ttl time.Duration) error {
	if ttl < time.Second {
		return fmt.Errorf("proxy [%s] ttl should be at least 1 second, got %s", name, ttl)
	}

	now := r.clk.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return fmt.Errorf("ttl registry is closed")
	}

	e, ok := r.entries[name]
	if ok {
		if e.expired {
			return &ExpiredError{Name: name, Expired: e.expiresAt}
		}
		// Existing live grant: keep the original deadline.
		return nil
	}

	e = &entry{
		name:      name,
		requested: ttl,
		expiresAt: now.Add(ttl),
	}
	r.entries[name] = e
	r.scheduleLocked(e)
	return nil
}

// Cancel removes the TTL record of a proxy. It is only used when a
// registration fails after Admit succeeded (rollback); a normal client
// initiated close keeps the record so the deadline continues to count down.
func (r *Registry) Cancel(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[name]; ok && !e.expired {
		if e.timer != nil {
			e.timer.Stop()
		}
		delete(r.entries, name)
	}
}

// Confirm re-checks the record after a successful registration. If the
// timer fired in the window between Admit and the proxy becoming live,
// Confirm returns false; the caller must roll the registration back.
func (r *Registry) Confirm(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[name]
	return ok && !e.expired
}

func (r *Registry) scheduleLocked(e *entry) {
	d := time.Until(e.expiresAt)
	if d <= 0 {
		d = time.Nanosecond
	}
	e.timer = r.clk.NewTimer(d)
	go func() {
		<-e.timer.C()
		r.fire(e.name)
	}()
}

func (r *Registry) fire(name string) {
	r.mu.Lock()
	e, ok := r.entries[name]
	if !ok || e.expired {
		r.mu.Unlock()
		return
	}
	e.expired = true
	e.timer = nil
	onExpire := r.onExpire
	r.mu.Unlock()

	log.Warnf("proxy [%s] ttl expired at %s (requested ttl %s), tearing down temporary tunnel",
		name, e.expiresAt.Format(time.RFC3339), e.requested)
	if onExpire != nil {
		onExpire(name)
	}
}

// Get returns a point-in-time view of the record for name.
func (r *Registry) Get(name string) (Info, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[name]
	if !ok {
		return Info{}, false
	}
	return r.infoLocked(e), true
}

// List returns point-in-time views of all records, sorted is the caller's
// responsibility.
func (r *Registry) List() []Info {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Info, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, r.infoLocked(e))
	}
	return out
}

func (r *Registry) infoLocked(e *entry) Info {
	info := Info{
		Name:         e.name,
		RequestedTTL: e.requested,
		ExpiresAt:    e.expiresAt,
		Expired:      e.expired,
	}
	if !e.expired {
		info.Remaining = e.expiresAt.Sub(r.clk.Now())
		if info.Remaining < 0 {
			info.Remaining = 0
		}
	}
	return info
}

// Close stops all pending timers. Firing of an already-expired timer is not
// rolled back.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	for _, e := range r.entries {
		if e.timer != nil {
			e.timer.Stop()
		}
	}
}
