package harness

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// NotificationRequest defines arguments for posting a test notification.
type NotificationRequest struct {
	Serial    string `json:"serial"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Channel   string `json:"channel,omitempty"`
	Tag       string `json:"tag,omitempty"`
	PackageID string `json:"package_id,omitempty"`
}

// PostNotification injects a system notification via Android cmd notification.
func PostNotification(ctx context.Context, dm types.DeviceManager, req NotificationRequest) (*types.NotificationPostResult, error) {
	if dm == nil {
		return nil, fmt.Errorf("device manager is required")
	}
	if req.Serial == "" {
		return nil, fmt.Errorf("device serial is required")
	}
	if req.Title == "" && req.Body == "" {
		return nil, fmt.Errorf("notification title or body is required")
	}

	// Create channel first if requested and package_id is provided
	if req.Channel != "" && req.PackageID != "" {
		channelName := req.Channel
		_, _ = dm.ExecADB(req.Serial, "shell", "cmd", "notification", "create_channel", req.PackageID, req.Channel, channelName)
	}

	tag := req.Tag
	if tag == "" {
		tag = "tag"
	}

	args := []string{"shell", "cmd", "notification", "post", "-S", "bigtext", "-t", req.Title}
	if req.Channel != "" {
		args = append(args, "-c", req.Channel)
	}
	args = append(args, tag, req.Body)

	output, err := dm.ExecADB(req.Serial, args...)
	if err != nil {
		return nil, fmt.Errorf("cmd notification post failed: %w (output: %s)", err, strings.TrimSpace(output))
	}

	return &types.NotificationPostResult{
		Posted: true,
		Method: "cmd_notification",
	}, nil
}
