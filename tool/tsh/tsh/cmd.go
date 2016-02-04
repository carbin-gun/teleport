/*
Copyright 2015 Gravitational, Inc.

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
package tsh

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/gravitational/teleport/lib/client"

	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"gopkg.in/alecthomas/kingpin.v2"
)

func RunTSH(app *kingpin.Application) error {
	sshAgentAddress := app.Flag("ssh-agent", "SSH agent address").OverrideDefaultFromEnvar("SSH_AUTH_SOCK").String()
	sshAgentNetwork := app.Flag("ssh-agent-network", "SSH agent address network type('tcp','unix' etc.)").Default("unix").String()
	webProxyAddress := app.Flag("web-proxy", "Web proxy address(used for login)").String()
	loginTTL := app.Flag("login-ttl", "Temporary ssh certificate will work for that time").Default("10h").Duration()

	connect := app.Command("ssh", "Connects to remote server and runs shell or provided command")
	connectAddress := connect.Arg("target", "Target server address. You can provide several servers using label searching target _label:value").Required().String()
	connectProxy := connect.Flag("proxy", "Optional proxy address").String()
	connectCommand := connect.Arg("command", "Run provided command instead of shell").String()
	connectPort := connect.Flag("port", "Remote server port").Short('p').String()

	getServers := app.Command("get-servers", "Returns list of servers")
	getServersProxy := getServers.Flag("proxy", "Target proxy address").Required().String()
	getServersLabelName := getServers.Flag("label", "Label name").String()
	getServersLabelValue := getServers.Flag("value", "Label value regexp").String()

	scp := app.Command("scp", "Copy file or files to the remote ssh server of from it")
	scpSource := scp.Arg("source", "source file or dir").Required().String()
	scpDest := scp.Arg("destination", "destination file or dir").Required().String()
	scpProxy := scp.Flag("proxy", "Optional proxy address").String()
	scpIsDir := scp.Flag("recursively", "Source path is a directory").Short('r').Bool()
	scpPort := scp.Flag("port", "Remote server port").Short('P').String()

	selectedCommand := kingpin.MustParse(app.Parse(os.Args[1:]))

	var user string
	switch selectedCommand {
	case connect.FullCommand():
		user, _ = client.SplitUserAndAddress(*connectAddress)
	case getServers.FullCommand():
		user, _ = client.SplitUserAndAddress(*getServersProxy)
	case scp.FullCommand():
		if strings.Contains(*scpSource, ":") {
			user, _ = client.SplitUserAndAddress(*scpSource)
		} else {
			user, _ = client.SplitUserAndAddress(*scpDest)
		}
	}
	if len(user) == 0 {
		return fmt.Errorf("Error: please provide user name")
	}

	standartSSHAgent, err := connectToSSHAgent(*sshAgentNetwork, *sshAgentAddress)
	if err != nil {
		return trace.Wrap(err)
	}
	teleportFileSSHAgent, err := client.GetLocalAgent()
	if err != nil {
		return trace.Wrap(err)
	}
	passwordCallback := client.GetPasswordFromConsole(user)

	authMethods := []ssh.AuthMethod{
		client.AuthMethodFromAgent(standartSSHAgent),
		client.AuthMethodFromAgent(teleportFileSSHAgent),
		client.NewWebAuth(
			teleportFileSSHAgent,
			user,
			passwordCallback,
			*webProxyAddress,
			*loginTTL,
		),
	}

	err = trace.Errorf("No command")

	switch selectedCommand {
	case connect.FullCommand():
		err = SSH(*connectAddress, *connectProxy, *connectCommand,
			*connectPort, authMethods)
	case getServers.FullCommand():
		err = GetServers(*getServersProxy, *getServersLabelName,
			*getServersLabelValue, authMethods)
	case scp.FullCommand():
		err = SCP(*scpProxy, *scpSource, *scpDest, *scpIsDir, *scpPort,
			authMethods)
	}

	return err
}

func connectToSSHAgent(network, address string) (agent.Agent, error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return agent.NewClient(conn), nil

}