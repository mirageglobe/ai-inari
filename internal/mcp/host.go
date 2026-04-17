package mcp

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/mirageglobe/ai-haniwa/internal/audit"
	"github.com/mirageglobe/ai-haniwa/internal/config"
)

// Connector wraps a running MCP child process.
type Connector struct {
	Name string
	cmd  *exec.Cmd
}

// Host manages all MCP connector child processes.
type Host struct {
	defs     []config.MCPConnector
	auditor  *audit.Auditor
	running  []*Connector
}

func NewHost(defs []config.MCPConnector, auditor *audit.Auditor) *Host {
	return &Host{defs: defs, auditor: auditor}
}

func (h *Host) Start() error {
	for _, def := range h.defs {
		cmd := exec.Command(def.Command, def.Args...)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("mcp: start %s: %w", def.Name, err)
		}
		h.running = append(h.running, &Connector{Name: def.Name, cmd: cmd})
	}
	return nil
}

func (h *Host) Stop() {
	for _, c := range h.running {
		c.cmd.Process.Kill()
	}
}

// Call routes a tool call to the named connector and logs it.
func (h *Host) Call(connector, method string, params any) error {
	raw, _ := json.Marshal(params)
	h.auditor.Log(fmt.Sprintf("mcp.%s.%s", connector, method), raw)
	// TODO: send JSON-RPC request over connector's stdio pipe
	return nil
}
