package mcphooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"nano_android/engine/compatibility/registry"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/media/mp4"
	"nano_android/engine/media/oracle"
	"nano_android/engine/profiler/crash"
	"nano_android/engine/security/pathcontainment"
	"nano_android/engine/testing/differential"
	"nano_android/engine/testing/labs"
)

// Attach wires all 12 specialized labs, 11-dimension differential testing, crash forensics/R8 retrace,
// and ISO BMFF + ffprobe media inspection handlers onto an mcp_compiler.ToolRegistry.
func Attach(reg *mcp_compiler.ToolRegistry, workspaceRoot string) {
	if reg == nil {
		return
	}
	allowed := []string{
		filepath.Join(workspaceRoot, "cache"),
		filepath.Join(workspaceRoot, "tools"),
		filepath.Join(workspaceRoot, "fixtures"),
		filepath.Join(workspaceRoot, "downloads"),
	}

	reg.SetPhase3Hooks(mcp_compiler.Phase3Hooks{
		ListLabs: func() any {
			return map[string]any{
				"labs":      labs.ListLabs(),
				"scenarios": labs.StressScenarios23(),
			}
		},
		RunLab: func(ctx context.Context, labID, artifactPath string, opts map[string]string) (any, error) {
			return labs.RunLab(ctx, workspaceRoot, labID, artifactPath, opts)
		},
		RunDifferential: func(ctx context.Context, args map[string]any) (any, error) {
			artA, _ := args["artifact_a"].(string)
			artB, _ := args["artifact_b"].(string)
			if artB == "" {
				artB = artA
			}
			pA, err := pathcontainment.ResolveExistingContained(artA, allowed, []string{".apk", ".jar"})
			if err != nil {
				return nil, fmt.Errorf("BLOCKED_POLICY artifact_a: %w", err)
			}
			pB, err := pathcontainment.ResolveExistingContained(artB, allowed, []string{".apk", ".jar"})
			if err != nil {
				return nil, fmt.Errorf("BLOCKED_POLICY artifact_b: %w", err)
			}
			backA, _ := args["backend_a"].(string)
			if backA == "" {
				backA = string(registry.BackendNanoJVM)
			}
			backB, _ := args["backend_b"].(string)
			if backB == "" {
				backB = string(registry.BackendNanoJVM)
			}
			profA, _ := args["profile_a"].(string)
			profB, _ := args["profile_b"].(string)
			modeStr, _ := args["mode"].(string)
			if modeStr == "" {
				modeStr = string(differential.ModeDeviceAVsDeviceB)
			}
			return differential.Run(ctx, differential.DifferentialRequest{
				Name:    "MCPDifferential",
				Mode:    differential.ComparisonMode(modeStr),
				TargetA: differential.TargetSpec{Label: "A", Backend: registry.Backend(backA), Artifact: pA, ProfileID: profA, APILevel: 35},
				TargetB: differential.TargetSpec{Label: "B", Backend: registry.Backend(backB), Artifact: pB, ProfileID: profB, APILevel: 35},
			}, nil, nil)
		},
		AnalyzeCrash: func(rawTrace, mappingPath, buildTag, envTier string) (any, error) {
			var mapping *crash.R8Mapping
			if mappingPath != "" {
				if raw, err := os.ReadFile(mappingPath); err == nil {
					mapping = crash.ParseR8Mapping(string(raw))
				}
			}
			return crash.AnalyzeTrace(rawTrace, buildTag, envTier, mapping), nil
		},
		InspectMedia: func(ctx context.Context, mediaPath string) (any, error) {
			rep, err := mp4.ParseFile(mediaPath)
			if err != nil {
				return nil, err
			}
			probe, _ := oracle.ProbeWithFFprobe(ctx, mediaPath)
			return map[string]any{"mp4": rep, "ffprobe": probe}, nil
		},
	})
}
