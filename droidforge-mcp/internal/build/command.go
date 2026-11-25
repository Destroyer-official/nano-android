package build

import (
	"context"
	"os/exec"
)

// CommandExecutor abstracts command execution for testability and mocking.
type CommandExecutor interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error)
}

// DefaultCommandExecutor runs commands on the host OS.
type DefaultCommandExecutor struct{}

// Run executes the command with context, directory, and environment.
func (e *DefaultCommandExecutor) Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = env
	}
	return cmd.CombinedOutput()
}

// MockCommandExecutor allows stubbing command outputs for unit testing.
type MockCommandExecutor struct {
	Handlers map[string]func(dir string, env []string, args []string) ([]byte, error)
	Calls    []MockCall
}

// MockCall records an executed command call.
type MockCall struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

// NewMockCommandExecutor creates a new mock command executor.
func NewMockCommandExecutor() *MockCommandExecutor {
	return &MockCommandExecutor{
		Handlers: make(map[string]func(dir string, env []string, args []string) ([]byte, error)),
	}
}

// RegisterHandler registers a mock response handler for a given command name.
func (m *MockCommandExecutor) RegisterHandler(name string, h func(dir string, env []string, args []string) ([]byte, error)) {
	m.Handlers[name] = h
}

// Run executes a registered handler or returns empty output.
func (m *MockCommandExecutor) Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	m.Calls = append(m.Calls, MockCall{
		Name: name,
		Args: args,
		Dir:  dir,
		Env:  env,
	})
	if h, ok := m.Handlers[name]; ok {
		return h(dir, env, args)
	}
	return nil, nil
}
