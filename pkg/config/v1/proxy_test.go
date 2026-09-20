// Copyright 2023 The frp Authors
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

package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fatedier/frp/pkg/msg"
)

func TestUnmarshalTypedProxyConfig(t *testing.T) {
	require := require.New(t)
	proxyConfigs := struct {
		Proxies []TypedProxyConfig `json:"proxies,omitempty"`
	}{}

	strs := `{
		"proxies": [
			{
				"type": "tcp",
				"localPort": 22,
				"remotePort": 6000
			},
			{
				"type": "http",
				"localPort": 80,
				"customDomains": ["www.example.com"]
			}
		]
	}`
	err := json.Unmarshal([]byte(strs), &proxyConfigs)
	require.NoError(err)

	require.IsType(&TCPProxyConfig{}, proxyConfigs.Proxies[0].ProxyConfigurer)
	require.IsType(&HTTPProxyConfig{}, proxyConfigs.Proxies[1].ProxyConfigurer)
}

func TestTTLJSONRoundTrip(t *testing.T) {
	require := require.New(t)
	cfg := &TCPProxyConfig{}
	require.NoError(json.Unmarshal([]byte(`{"type":"tcp","name":"p","remotePort":6000,"ttl":"2h"}`), cfg))
	require.Equal(int64(2*60*60), cfg.TTL.Seconds())

	b, err := json.Marshal(cfg)
	require.NoError(err)
	require.Contains(string(b), `"ttl":"2h0m0s"`)
}

func TestTTLInvalidDurationRejectedAtUnmarshal(t *testing.T) {
	require := require.New(t)
	cfg := &TCPProxyConfig{}
	err := json.Unmarshal([]byte(`{"type":"tcp","name":"p","remotePort":6000,"ttl":"forever"}`), cfg)
	require.Error(err)
}

func TestTTLMarshalToMsg(t *testing.T) {
	require := require.New(t)
	cfg := &TCPProxyConfig{}
	require.NoError(json.Unmarshal([]byte(`{"type":"tcp","name":"p","remotePort":6000,"ttl":"90m"}`), cfg))
	m := &msg.NewProxy{}
	cfg.MarshalToMsg(m)
	require.Equal(int64(5400), m.TTLSeconds)

	got := &TCPProxyConfig{}
	got.UnmarshalFromMsg(m)
	require.Equal(int64(5400), got.TTL.Seconds())
}
