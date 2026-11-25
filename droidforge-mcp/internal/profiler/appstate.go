package profiler

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

var (
	reServiceRecord   = regexp.MustCompile(`ServiceRecord\{[0-9a-fA-F]+\s+[a-zA-Z0-9._/]+\s+([^\s}]+)`)
	reRuntimePermLine = regexp.MustCompile(`(?i)([a-zA-Z0-9._]+):\s*granted=(true|false)`)
	reJobBlock        = regexp.MustCompile(`JOB #(\d+):\s*([^\n\r]+)`)
	reJobTag          = regexp.MustCompile(`(?i)tag=([^\s,]+)`)
	reStandbyBucket   = regexp.MustCompile(`(?i)(?:standby_bucket|bucket)[=:\s]+([A-Z_]+|\d+)`)
)

// AppStateRequest holds options for inspecting app state.
type AppStateRequest struct {
	Serial    string   `json:"serial"`
	PackageID string   `json:"package_id"`
	Sections  []string `json:"sections,omitempty"` // services, permissions, workmanager, standby_bucket
}

// ParseServices extracts service names from dumpsys activity services output.
func ParseServices(output, pkg string) []string {
	services := make([]string, 0)
	matches := reServiceRecord.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		if len(m) > 1 {
			srv := m[1]
			if pkg == "" || strings.Contains(srv, pkg) {
				services = append(services, srv)
			}
		}
	}
	return services
}

// ParsePermissions extracts granted and denied permissions from dumpsys package output.
func ParsePermissions(output string) (granted []string, denied []string) {
	granted = make([]string, 0)
	denied = make([]string, 0)

	matches := reRuntimePermLine.FindAllStringSubmatch(output, -1)
	seen := make(map[string]bool)

	for _, m := range matches {
		if len(m) > 2 {
			permName := m[1]
			isGranted := strings.ToLower(m[2]) == "true"
			if seen[permName] {
				continue
			}
			seen[permName] = true
			if isGranted {
				granted = append(granted, permName)
			} else {
				denied = append(denied, permName)
			}
		}
	}
	return granted, denied
}

// ParseStandbyBucket maps standby bucket IDs or strings to standard names.
func ParseStandbyBucket(output string) string {
	raw := strings.TrimSpace(output)
	if match := reStandbyBucket.FindStringSubmatch(raw); len(match) > 1 {
		raw = match[1]
	}

	upper := strings.ToUpper(raw)
	switch upper {
	case "10", "ACTIVE":
		return "ACTIVE"
	case "20", "WORKING_SET":
		return "WORKING_SET"
	case "30", "FREQUENT":
		return "FREQUENT"
	case "40", "RARE":
		return "RARE"
	case "45", "RESTRICTED":
		return "RESTRICTED"
	case "50", "NEVER":
		return "NEVER"
	default:
		if upper == "" {
			return "UNKNOWN"
		}
		return upper
	}
}

// ParseWorkManagerJobs extracts scheduled jobs from dumpsys jobscheduler for the target package.
func ParseWorkManagerJobs(output, pkg string) []types.WorkManagerJob {
	jobs := make([]types.WorkManagerJob, 0)
	lines := strings.Split(output, "\n")

	var currentJob *types.WorkManagerJob

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")

		// Check new job block
		if match := reJobBlock.FindStringSubmatch(trimmed); len(match) > 2 {
			jobID := match[1]
			jobHeader := match[2]
			if pkg == "" || strings.Contains(jobHeader, pkg) {
				if currentJob != nil {
					jobs = append(jobs, *currentJob)
				}
				currentJob = &types.WorkManagerJob{
					ID:    jobID,
					State: "SCHEDULED",
					Tags:  make([]string, 0),
				}
			} else {
				currentJob = nil
			}
			continue
		}

		if currentJob != nil {
			if strings.Contains(trimmed, "Ready=") || strings.Contains(trimmed, "satisfied") {
				currentJob.State = "READY"
			}
			if match := reJobTag.FindStringSubmatch(trimmed); len(match) > 1 {
				currentJob.Tags = append(currentJob.Tags, match[1])
			}
		}
	}

	if currentJob != nil {
		jobs = append(jobs, *currentJob)
	}

	return jobs
}

// InspectAppState executes strictly allowlisted dumpsys queries to inspect app state.
func InspectAppState(ctx context.Context, dm types.DeviceManager, req AppStateRequest) (*types.AppStateResult, error) {
	if dm == nil {
		return nil, fmt.Errorf("device manager is required")
	}
	if req.Serial == "" {
		return nil, fmt.Errorf("device serial is required")
	}
	if req.PackageID == "" {
		return nil, fmt.Errorf("package_id is required for app state inspection")
	}
	if !ValidatePackageID(req.PackageID) {
		return nil, fmt.Errorf("invalid package_id %q: must match ^[a-zA-Z0-9._]+$", req.PackageID)
	}

	sections := req.Sections
	if len(sections) == 0 {
		sections = []string{"services", "permissions", "workmanager", "standby_bucket"}
	}

	result := &types.AppStateResult{
		Nonce: GenerateCryptoNonce(),
	}

	for _, s := range sections {
		section := strings.ToLower(strings.TrimSpace(s))
		switch section {
		case "services":
			// Allowlisted: dumpsys activity services <pkg>
			out, err := dm.ExecADB(req.Serial, "shell", "dumpsys", "activity", "services", req.PackageID)
			if err == nil {
				result.RunningServices = ParseServices(out, req.PackageID)
			}

		case "permissions":
			// Allowlisted: dumpsys package <pkg>
			out, err := dm.ExecADB(req.Serial, "shell", "dumpsys", "package", req.PackageID)
			if err == nil {
				granted, denied := ParsePermissions(out)
				result.GrantedPerms = granted
				result.DeniedPerms = denied
			}

		case "standby_bucket":
			// Allowlisted: am get-standby-bucket <pkg>
			out, err := dm.ExecADB(req.Serial, "shell", "am", "get-standby-bucket", req.PackageID)
			if err == nil {
				result.StandbyBucket = ParseStandbyBucket(out)
			}

		case "workmanager", "jobscheduler":
			// Allowlisted: dumpsys jobscheduler
			out, err := dm.ExecADB(req.Serial, "shell", "dumpsys", "jobscheduler", req.PackageID)
			if err == nil {
				result.WorkManagerJobs = ParseWorkManagerJobs(out, req.PackageID)
			}

		default:
			return nil, fmt.Errorf("section %q is not in allowlisted app state inspection queries", section)
		}
	}

	return result, nil
}
