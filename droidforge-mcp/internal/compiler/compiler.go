package compiler

import (
	"context"
	"fmt"
	"sync"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// Compiler coordinates session recording, test freezing, execution, and appcard export.
// It implements types.TestCompiler.
type Compiler struct {
	mu           sync.RWMutex
	recorders    map[string]*SessionRecorder
	runner       *MaestroRunner
	defaultAppID string
}

// NewCompiler creates a new test compiler instance.
func NewCompiler(runner *MaestroRunner) *Compiler {
	if runner == nil {
		runner = NewMaestroRunner(nil)
	}
	return &Compiler{
		recorders: make(map[string]*SessionRecorder),
		runner:    runner,
	}
}

// SetDefaultAppID sets the fallback app package ID for flows and appcards.
func (c *Compiler) SetDefaultAppID(appID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.defaultAppID = appID
}

// GetOrCreateRecorder returns the recorder for a session ID, creating one if needed.
func (c *Compiler) GetOrCreateRecorder(sessionID string) *SessionRecorder {
	c.mu.Lock()
	defer c.mu.Unlock()

	rec, exists := c.recorders[sessionID]
	if !exists {
		rec = NewSessionRecorder(sessionID)
		if c.defaultAppID != "" {
			rec.SetAppID(c.defaultAppID)
		}
		c.recorders[sessionID] = rec
	}
	return rec
}

// RecordStep appends an action step to the active session buffer.
func (c *Compiler) RecordStep(sessionID string, step types.SessionStep) {
	rec := c.GetOrCreateRecorder(sessionID)
	rec.RecordStep(step)
}

// FreezeToMaestro compiles the net successful path into a Maestro YAML flow.
func (c *Compiler) FreezeToMaestro(sessionID, flowName, outputPath string) (*types.FreezeResult, error) {
	c.mu.RLock()
	rec, exists := c.recorders[sessionID]
	c.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no recorded exploration session found with ID %q", sessionID)
	}

	return FreezeSessionToMaestro(rec, flowName, outputPath)
}

// RunMaestro executes a Maestro YAML flow with optional burn-in.
func (c *Compiler) RunMaestro(serial, flowPath string, repeatCount int) (*types.MaestroRunResult, error) {
	return c.runner.Run(context.Background(), serial, flowPath, repeatCount)
}

// ExportAppCard generates an AppCard markdown file from session data.
func (c *Compiler) ExportAppCard(sessionID, outputPath string) (*types.AppCardResult, error) {
	c.mu.RLock()
	rec, exists := c.recorders[sessionID]
	c.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no recorded exploration session found with ID %q", sessionID)
	}

	return ExportAppCard(rec, outputPath)
}
