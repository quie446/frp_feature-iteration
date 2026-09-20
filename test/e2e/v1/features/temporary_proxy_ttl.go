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

package features

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/fatedier/frp/test/e2e/framework"
	"github.com/fatedier/frp/test/e2e/pkg/request"
)

type proxyTTLAPIResp struct {
	Name             string `json:"name"`
	Status           string `json:"status"`
	TTL              string `json:"ttl"`
	Expired          bool   `json:"expired"`
	RemainingSeconds int64  `json:"remainingSeconds"`
}

var _ = ginkgo.Describe("[Feature: TemporaryProxyTTL]", func() {
	f := framework.NewDefaultFramework()

	ginkgo.It("tears down a temporary tcp proxy on ttl expiry and rejects re-registration", func() {
		serverPort := f.AllocPort()
		dashboardPort := f.AllocPort()
		remotePort := f.AllocPort()

		serverConf := fmt.Sprintf(`
		bindAddr = "127.0.0.1"
		bindPort = %d
		log.level = "trace"
		webServer.addr = "127.0.0.1"
		webServer.port = %d
		webServer.user = "admin"
		webServer.password = "admin"
		`, serverPort, dashboardPort)

		clientConf := fmt.Sprintf(`
		serverAddr = "127.0.0.1"
		serverPort = %d
		log.level = "trace"
		transport.tls.enable = false

		[[proxies]]
		name = "temp-tcp"
		type = "tcp"
		localPort = {{ .%s }}
		remotePort = %d
		ttl = "3s"
		`, serverPort, framework.TCPEchoServerPort, remotePort)

		serverProc, clients := f.RunProcesses(serverConf, []string{clientConf})
		clientProc := clients[0]
		clientConfigPath := f.ClientConfigPath(0)

		// The tunnel works while alive.
		framework.NewRequestExpect(f).Protocol("tcp").Port(remotePort).Ensure()

		// The admin API reports the remaining lifetime.
		remaining := getProxyTTLInfo(f, dashboardPort, "temp-tcp")
		framework.ExpectEqual(remaining.TTL, "3s")
		framework.ExpectEqual(remaining.Expired, false)

		// Wait well past the TTL.
		time.Sleep(4 * time.Second)

		// The tunnel is gone.
		framework.NewRequestExpect(f).Protocol("tcp").Port(remotePort).
			ExpectError(true).Ensure()

		// frps logged the expiry and frpc stopped the proxy after the
		// server pushed CloseProxy instead of silently reviving it.
		framework.ExpectNoError(serverProc.WaitForOutput("ttl expired", 1, 2*time.Second))
		framework.ExpectNoError(clientProc.WaitForOutput("server requested to close proxy [temp-tcp]", 1, 2*time.Second))

		// The admin API marks the name as expired.
		expired := getProxyTTLInfo(f, dashboardPort, "temp-tcp")
		framework.ExpectEqual(expired.Status, "offline")
		framework.ExpectEqual(expired.Expired, true)

		// A fresh client registering the same name must be rejected.
		newClientProc, _, err := f.StartFrpc("-c", clientConfigPath)
		framework.ExpectNoError(err)
		framework.ExpectNoError(newClientProc.WaitForOutput("is expired", 1, 3*time.Second))
		framework.NewRequestExpect(f).Protocol("tcp").Port(remotePort).
			ExpectError(true).Ensure()
	})
})

func getProxyTTLInfo(f *framework.Framework, dashboardPort int, name string) proxyTTLAPIResp {
	var result proxyTTLAPIResp
	framework.NewRequestExpect(f).
		RequestModify(func(r *request.Request) {
			r.HTTP().
				Port(dashboardPort).
				HTTPAuth("admin", "admin").
				HTTPPath(fmt.Sprintf("/api/proxies/%s", name))
		}).
		Ensure(func(resp *request.Response) bool {
			if resp.Code != http.StatusOK {
				return false
			}
			return json.Unmarshal(resp.Content, &result) == nil
		})
	return result
}
