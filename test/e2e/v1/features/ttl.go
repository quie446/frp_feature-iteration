package features

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/fatedier/frp/test/e2e/framework"
	"github.com/fatedier/frp/test/e2e/framework/consts"
)

var _ = ginkgo.Describe("[Feature: Proxy TTL]", func() {
	f := framework.NewDefaultFramework()

	ginkgo.It("tcp proxy is closed by server after TTL expires", func() {
		serverConf := consts.DefaultServerConfig

		remotePort := f.AllocPort()
		clientConf := fmt.Sprintf(`
		%s

		[[proxies]]
		name = "ttl-tcp"
		type = "tcp"
		localPort = %d
		remotePort = %d
		ttlSeconds = 3
		`, consts.DefaultClientConfig, f.PortByName(framework.TCPEchoServerPort), remotePort)

		f.RunProcesses(serverConf, []string{clientConf})

		// the tunnel works before the TTL expires
		framework.NewRequestExpect(f).Protocol("tcp").Port(remotePort).Ensure()

		// wait for the TTL to elapse
		time.Sleep(6 * time.Second)

		// the server has torn the tunnel down, requests now fail
		framework.NewRequestExpect(f).Protocol("tcp").Port(remotePort).ExpectError(true).Ensure()
	})

	ginkgo.It("proxy without TTL keeps working", func() {
		serverConf := consts.DefaultServerConfig

		remotePort := f.AllocPort()
		clientConf := fmt.Sprintf(`
		%s

		[[proxies]]
		name = "no-ttl-tcp"
		type = "tcp"
		localPort = %d
		remotePort = %d
		`, consts.DefaultClientConfig, f.PortByName(framework.TCPEchoServerPort), remotePort)

		f.RunProcesses(serverConf, []string{clientConf})

		framework.NewRequestExpect(f).Protocol("tcp").Port(remotePort).Ensure()

		// no TTL configured, the tunnel is not affected
		time.Sleep(6 * time.Second)
		framework.NewRequestExpect(f).Protocol("tcp").Port(remotePort).Ensure()
	})
})
