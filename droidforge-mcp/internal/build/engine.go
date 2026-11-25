package build

import (
	"context"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// Ensure GradleEngine implements types.BuildEngine at compile-time.
var _ types.BuildEngine = (*GradleEngine)(nil)

// Option configures a GradleEngine instance.
type Option func(*GradleEngine)

// WithExecutor configures a custom command executor (e.g. for testing).
func WithExecutor(executor CommandExecutor) Option {
	return func(g *GradleEngine) {
		if executor != nil {
			g.executor = executor
		}
	}
}

// WithBuildToolsDir sets the path to the Android SDK build-tools directory.
func WithBuildToolsDir(dir string) Option {
	return func(g *GradleEngine) {
		g.buildToolsDir = dir
	}
}

// GradleEngine implements types.BuildEngine to orchestrate Gradle builds,
// Robolectric/Roborazzi tests, APK static intake, and compliance audits.
type GradleEngine struct {
	gradleRoot    string
	maxHeapMB     int
	executor      CommandExecutor
	buildToolsDir string
}

// NewGradleEngine creates a new GradleEngine for the given project root.
func NewGradleEngine(gradleRoot string, maxHeapMB int, opts ...Option) *GradleEngine {
	if maxHeapMB <= 0 {
		maxHeapMB = 2048
	}
	if gradleRoot == "" {
		gradleRoot = "."
	}

	engine := &GradleEngine{
		gradleRoot: gradleRoot,
		maxHeapMB:  maxHeapMB,
		executor:   &DefaultCommandExecutor{},
	}

	for _, opt := range opts {
		opt(engine)
	}

	return engine
}

// Build triggers a Gradle build and returns structured APK metadata and errors.
func (b *GradleEngine) Build(variant, module string, clean bool) (*types.BuildResult, error) {
	return b.executeBuild(context.Background(), variant, module, clean)
}

// BuildWithContext triggers a Gradle build within the provided context.
func (b *GradleEngine) BuildWithContext(ctx context.Context, variant, module string, clean bool) (*types.BuildResult, error) {
	return b.executeBuild(ctx, variant, module, clean)
}

// TestJVM runs Robolectric/Compose JVM tests and parses JUnit XML results.
func (b *GradleEngine) TestJVM(filter string, roborazzi, recordGoldens bool) (*types.TestResult, error) {
	return b.executeTestJVM(context.Background(), filter, roborazzi, recordGoldens)
}

// TestJVMWithContext runs Robolectric/Compose JVM tests within the provided context.
func (b *GradleEngine) TestJVMWithContext(ctx context.Context, filter string, roborazzi, recordGoldens bool) (*types.TestResult, error) {
	return b.executeTestJVM(ctx, filter, roborazzi, recordGoldens)
}

// InspectAPK performs static analysis on an APK without source code.
func (b *GradleEngine) InspectAPK(apkPath string, decompileClasses []string) (*types.APKInfo, error) {
	return b.executeInspectAPK(context.Background(), apkPath, decompileClasses)
}

// InspectAPKWithContext performs static analysis on an APK within the provided context.
func (b *GradleEngine) InspectAPKWithContext(ctx context.Context, apkPath string, decompileClasses []string) (*types.APKInfo, error) {
	return b.executeInspectAPK(ctx, apkPath, decompileClasses)
}

// AuditCompliance checks Play policy and Android 17 compatibility.
func (b *GradleEngine) AuditCompliance(apkPath, profile string) (*types.ComplianceResult, error) {
	return b.executeAuditCompliance(context.Background(), apkPath, profile)
}

// AuditComplianceWithContext checks Play policy and Android 17 compatibility within the provided context.
func (b *GradleEngine) AuditComplianceWithContext(ctx context.Context, apkPath, profile string) (*types.ComplianceResult, error) {
	return b.executeAuditCompliance(ctx, apkPath, profile)
}
