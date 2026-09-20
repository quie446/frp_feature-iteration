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

package http

import (
	"net/http"
	"testing"
	"time"

	clocktesting "k8s.io/utils/clock/testing"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/metrics/mem"
	serverproxy "github.com/fatedier/frp/server/proxy"
	serverttl "github.com/fatedier/frp/server/ttl"
)

func TestAPIV2ProxyTTLFields(t *testing.T) {
	oldStatsCollector := mem.StatsCollector
	mem.StatsCollector = &fakeStatsCollector{
		proxies: map[string]*mem.ProxyStats{
			"temp": {Name: "temp", Type: "tcp"},
		},
	}
	t.Cleanup(func() { mem.StatsCollector = oldStatsCollector })

	fixed := clocktesting.NewFakeClock(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	registry := serverttl.NewRegistryWithClockForTest(fixed, nil)
	if err := registry.Admit("temp", time.Hour); err != nil {
		t.Fatalf("admit ttl: %v", err)
	}

	controller := NewController(&v1.ServerConfig{}, nil, serverproxy.NewManager(), registry)
	router := newV2TestRouter(controller)

	resp := performRequest(router, "/api/v2/proxies/temp")
	if resp.Code != http.StatusOK {
		t.Fatalf("status mismatch, want %d got %d, body: %s", http.StatusOK, resp.Code, resp.Body.String())
	}
	data := decodeResponse[v2EnvelopeForTest[map[string]any]](t, resp).Data
	status, ok := data["status"].(map[string]any)
	if !ok {
		t.Fatalf("status missing: %#v", data)
	}
	if expired, present := status["expired"]; present && expired != false {
		t.Fatalf("unexpected expired: %#v", status)
	}
	if remaining, _ := status["remainingSeconds"].(float64); remaining < 3599 || remaining > 3600 {
		t.Fatalf("remainingSeconds mismatch: %v", status["remainingSeconds"])
	}
	if int64(status["expiresAt"].(float64)) != fixed.Now().Add(time.Hour).Unix() {
		t.Fatalf("expiresAt mismatch: %v", status["expiresAt"])
	}
}
