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

package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func int64Ptr(v int64) *int64 {
	return &v
}

func TestValidateProxyTTLSeconds(t *testing.T) {
	newTCPProxy := func(ttl *int64) *v1.TCPProxyConfig {
		return &v1.TCPProxyConfig{
			ProxyBaseConfig: v1.ProxyBaseConfig{
				Name:       "test",
				Type:       "tcp",
				TTLSeconds: ttl,
				ProxyBackend: v1.ProxyBackend{
					LocalIP:   "127.0.0.1",
					LocalPort: 8080,
				},
			},
			RemotePort: 8080,
		}
	}

	tests := []struct {
		name    string
		ttl     *int64
		wantErr bool
	}{
		{name: "unset means no TTL", ttl: nil},
		{name: "positive TTL", ttl: int64Ptr(7200)},
		{name: "zero TTL rejected", ttl: int64Ptr(0), wantErr: true},
		{name: "negative TTL rejected", ttl: int64Ptr(-1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newTCPProxy(tt.ttl)
			cfg.Complete()
			err := ValidateProxyConfigurerForClient(cfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			err = ValidateProxyConfigurerForServer(cfg, &v1.ServerConfig{})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateDomainConfigForServerRejectsSubdomainHostCaseInsensitively(t *testing.T) {
	tests := []struct {
		name          string
		subDomainHost string
		customDomain  string
		wantErr       bool
	}{
		{
			name:          "lowercase subdomain",
			subDomainHost: "frp.example.com",
			customDomain:  "victim.frp.example.com",
			wantErr:       true,
		},
		{
			name:          "mixed case custom domain",
			subDomainHost: "frp.example.com",
			customDomain:  "victim.FRP.example.com",
			wantErr:       true,
		},
		{
			name:          "mixed case wildcard domain",
			subDomainHost: "frp.example.com",
			customDomain:  "*.FRP.example.com",
			wantErr:       true,
		},
		{
			name:          "mixed case subdomain host",
			subDomainHost: "FRP.Example.Com",
			customDomain:  "victim.frp.example.com",
			wantErr:       true,
		},
		{
			name:          "external domain",
			subDomainHost: "frp.example.com",
			customDomain:  "victim.example.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDomainConfigForServer(
				&v1.DomainConfig{CustomDomains: []string{tt.customDomain}},
				&v1.ServerConfig{SubDomainHost: tt.subDomainHost},
			)
			if tt.wantErr {
				require.ErrorContains(t, err, "should not belong to subdomain host")
				return
			}
			require.NoError(t, err)
		})
	}
}
