package harness

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// AppOp defines an operation and its mode for appops.
type AppOp struct {
	Op   string `json:"op"`
	Mode string `json:"mode"`
}

// PermissionsRequest holds parameters for permission modifications and system simulation.
type PermissionsRequest struct {
	Serial        string   `json:"serial"`
	PackageID     string   `json:"package_id"`
	Grants        []string `json:"grants,omitempty"`
	Revokes       []string `json:"revokes,omitempty"`
	AppOps        []AppOp  `json:"appops,omitempty"`
	StandbyBucket string   `json:"standby_bucket,omitempty"` // active, working_set, frequent, rare, restricted, never
	DozeMode      string   `json:"doze_mode,omitempty"`      // force-idle, unforce
}

// SetPermissions modifies app permissions, appops, standby bucket, and deviceidle doze state.
func SetPermissions(ctx context.Context, dm types.DeviceManager, req PermissionsRequest) (*types.PermissionSetResult, error) {
	if dm == nil {
		return nil, fmt.Errorf("device manager is required")
	}
	if req.Serial == "" {
		return nil, fmt.Errorf("device serial is required")
	}

	applied := make([]string, 0)
	errors := make([]string, 0)

	// Validate package_id to prevent flag injection (e.g. --user 0 com.android.settings)
	if req.PackageID != "" && !ValidateIdentifier(req.PackageID) {
		return nil, fmt.Errorf("invalid package_id %q: must match ^[a-zA-Z0-9._]+$", req.PackageID)
	}

	// 1. Grants
	for _, perm := range req.Grants {
		perm = strings.TrimSpace(perm)
		if perm == "" {
			continue
		}
		if req.PackageID == "" {
			errors = append(errors, fmt.Sprintf("grant %s: package_id is required", perm))
			continue
		}
		if !ValidateIdentifier(perm) {
			errors = append(errors, fmt.Sprintf("grant %s: invalid permission name", perm))
			continue
		}
		out, err := dm.ExecADB(req.Serial, "shell", "pm", "grant", req.PackageID, perm)
		if err != nil {
			errors = append(errors, fmt.Sprintf("grant %s: %v (%s)", perm, err, strings.TrimSpace(out)))
		} else {
			applied = append(applied, fmt.Sprintf("pm grant %s", perm))
		}
	}

	// 2. Revokes
	for _, perm := range req.Revokes {
		perm = strings.TrimSpace(perm)
		if perm == "" {
			continue
		}
		if req.PackageID == "" {
			errors = append(errors, fmt.Sprintf("revoke %s: package_id is required", perm))
			continue
		}
		if !ValidateIdentifier(perm) {
			errors = append(errors, fmt.Sprintf("revoke %s: invalid permission name", perm))
			continue
		}
		out, err := dm.ExecADB(req.Serial, "shell", "pm", "revoke", req.PackageID, perm)
		if err != nil {
			errors = append(errors, fmt.Sprintf("revoke %s: %v (%s)", perm, err, strings.TrimSpace(out)))
		} else {
			applied = append(applied, fmt.Sprintf("pm revoke %s", perm))
		}
	}

	// 3. AppOps
	for _, op := range req.AppOps {
		opName := strings.TrimSpace(op.Op)
		opMode := strings.TrimSpace(op.Mode)
		if opName == "" || opMode == "" {
			continue
		}
		if req.PackageID == "" {
			errors = append(errors, fmt.Sprintf("appops %s: package_id is required", opName))
			continue
		}
		if !ValidateIdentifier(opName) || !ValidateIdentifier(opMode) {
			errors = append(errors, fmt.Sprintf("appops %s %s: invalid identifier", opName, opMode))
			continue
		}
		out, err := dm.ExecADB(req.Serial, "shell", "appops", "set", req.PackageID, opName, opMode)
		if err != nil {
			errors = append(errors, fmt.Sprintf("appops set %s %s: %v (%s)", opName, opMode, err, strings.TrimSpace(out)))
		} else {
			applied = append(applied, fmt.Sprintf("appops set %s %s", opName, opMode))
		}
	}

	// 4. Standby Bucket
	if req.StandbyBucket != "" {
		bucket := strings.ToLower(strings.TrimSpace(req.StandbyBucket))
		if req.PackageID == "" {
			errors = append(errors, "standby_bucket: package_id is required")
		} else {
			out, err := dm.ExecADB(req.Serial, "shell", "am", "set-standby-bucket", req.PackageID, bucket)
			if err != nil {
				errors = append(errors, fmt.Sprintf("set-standby-bucket %s: %v (%s)", bucket, err, strings.TrimSpace(out)))
			} else {
				applied = append(applied, fmt.Sprintf("am set-standby-bucket %s", bucket))
			}
		}
	}

	// 5. Doze Mode Simulation
	if req.DozeMode != "" {
		mode := strings.ToLower(strings.TrimSpace(req.DozeMode))
		switch mode {
		case "force-idle", "idle", "force":
			out, err := dm.ExecADB(req.Serial, "shell", "dumpsys", "deviceidle", "force-idle")
			if err != nil {
				errors = append(errors, fmt.Sprintf("deviceidle force-idle: %v (%s)", err, strings.TrimSpace(out)))
			} else {
				applied = append(applied, "dumpsys deviceidle force-idle")
			}
		case "unforce", "unidle", "resume":
			out, err := dm.ExecADB(req.Serial, "shell", "dumpsys", "deviceidle", "unforce")
			if err != nil {
				errors = append(errors, fmt.Sprintf("deviceidle unforce: %v (%s)", err, strings.TrimSpace(out)))
			} else {
				applied = append(applied, "dumpsys deviceidle unforce")
			}
		default:
			errors = append(errors, fmt.Sprintf("unknown doze mode %q (supported: force-idle, unforce)", mode))
		}
	}

	return &types.PermissionSetResult{
		Applied: applied,
		Errors:  errors,
	}, nil
}
