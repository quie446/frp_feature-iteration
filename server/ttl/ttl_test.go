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

package ttl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/clock"
	testingclock "k8s.io/utils/clock/testing"
)

func TestAdmitRejectsInvalidTTL(t *testing.T) {
	r := NewRegistry(nil)
	defer r.Close()

	require.Error(t, r.Admit("zero", 0))
	require.Error(t, r.Admit("negative", -time.Second))
	require.Error(t, r.Admit("sub-second", 500*time.Millisecond))
}

func TestReconnectDoesNotExtendDeadline(t *testing.T) {
	clk := testingclock.NewFakeClock(time.Now())
	expired := make(chan string, 1)
	r := newRegistryWithClock(clk, func(name string) {
		expired <- name
	})
	defer r.Close()

	require.NoError(t, r.Admit("temp", 2*time.Second))

	clk.Step(time.Second)
	// Client reconnects with a longer TTL: original deadline must win.
	require.NoError(t, r.Admit("temp", 10*time.Hour))
	info, ok := r.Get("temp")
	require.True(t, ok)
	require.False(t, info.Expired)
	require.Equal(t, time.Second, info.Remaining)

	clk.Step(time.Second)
	select {
	case name := <-expired:
		require.Equal(t, "temp", name)
	case <-time.After(time.Second):
		t.Fatal("expected expiry callback")
	}

	info, ok = r.Get("temp")
	require.True(t, ok)
	require.True(t, info.Expired)

	// Re-registration under the same name must fail.
	err := r.Admit("temp", time.Hour)
	require.Error(t, err)
	var expiredErr *ExpiredError
	require.ErrorAs(t, err, &expiredErr)
	require.Equal(t, "temp", expiredErr.Name)
}

func TestCancelAllowsFreshGrant(t *testing.T) {
	clk := testingclock.NewFakeClock(time.Now())
	r := newRegistryWithClock(clk, nil)
	defer r.Close()

	require.NoError(t, r.Admit("temp", time.Minute))
	r.Cancel("temp")

	// After cancel (registration rollback), a fresh grant is accepted.
	require.NoError(t, r.Admit("temp", time.Minute))
	info, ok := r.Get("temp")
	require.True(t, ok)
	require.Equal(t, time.Minute, info.Remaining)
}

func TestExpiredRecordSurvivesReconnectAttempts(t *testing.T) {
	clk := testingclock.NewFakeClock(time.Now())
	expired := make(chan struct{}, 1)
	r := newRegistryWithClock(clk, func(string) {
		expired <- struct{}{}
	})
	defer r.Close()

	require.NoError(t, r.Admit("temp", time.Second))
	clk.Step(time.Second)
	<-expired

	// No TTL at all still cannot hijack the expired name.
	require.NoError(t, r.Admit("other", time.Second))
	err := r.Admit("temp", 0)
	require.Error(t, err)
}

var _ clock.PassiveClock = (*testingclock.FakeClock)(nil)
