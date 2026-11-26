package ide

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nano_android/engine/security/pathcontainment"
)

// IDEClient enumerates supported IDE and AI coding client integrations (§10.1).
type IDEClient string

const (
	ClientAndroidStudio IDEClient = "android_studio"
	ClientIntelliJIDEA  IDEClient = "intellij_idea"
	ClientVSCode        IDEClient = "vscode"
	ClientCursor        IDEClient = "cursor"
	ClientAntigravity   IDEClient = "antigravity"
	ClientClaudeCode    IDEClient = "claude_code"
	ClientGenericMCP    IDEClient = "generic_mcp"
)

// IDEIntegrationBundle contains ready-to-install configuration payloads for an IDE/MCP host.
type IDEIntegrationBundle struct {
	Client        IDEClient `json:"client"`
	DisplayName   string    `json:"display_name"`
	ConfigRelPath string    `json:"config_rel_path"`
	Format        string    `json:"format"` // "json" or "xml"
	Content       string    `json:"content"`
	InstallNote   string    `json:"install_note"`
}

// ListClients lists all 7 supported IDE / MCP client integrations.
func ListClients() []IDEClient {
	return []IDEClient{
		ClientAndroidStudio,
		ClientIntelliJIDEA,
		ClientVSCode,
		ClientCursor,
		ClientAntigravity,
		ClientClaudeCode,
		ClientGenericMCP,
	}
}

// GenerateConfig renders a valid IDE/MCP client configuration invoking the Nano-Android MCP binary cleanly.
func GenerateConfig(client IDEClient, mcpBinaryPath, workspaceRoot string) (IDEIntegrationBundle, error) {
	if strings.TrimSpace(mcpBinaryPath) == "" {
		mcpBinaryPath = "nano"
	}
	args := []string{"server"}
	if strings.Contains(strings.ToLower(filepath.Base(mcpBinaryPath)), "nano-android-mcp") {
		args = []string{}
	}

	mcpEntry := map[string]any{
		"command": mcpBinaryPath,
		"args":    args,
		"cwd":     workspaceRoot,
	}

	switch client {
	case ClientVSCode:
		b, _ := json.MarshalIndent(map[string]any{
			"servers": map[string]any{"nano-android": mcpEntry},
		}, "", "  ")
		return IDEIntegrationBundle{
			Client:        client,
			DisplayName:   "Visual Studio Code (.vscode/mcp.json)",
			ConfigRelPath: filepath.Join(".vscode", "mcp.json"),
			Format:        "json",
			Content:       string(b) + "\n",
			InstallNote:   "Place in workspace .vscode/mcp.json and reload VS Code MCP extension.",
		}, nil
	case ClientCursor:
		b, _ := json.MarshalIndent(map[string]any{
			"mcpServers": map[string]any{"nano-android": mcpEntry},
		}, "", "  ")
		return IDEIntegrationBundle{
			Client:        client,
			DisplayName:   "Cursor (.cursor/mcp.json)",
			ConfigRelPath: filepath.Join(".cursor", "mcp.json"),
			Format:        "json",
			Content:       string(b) + "\n",
			InstallNote:   "Place in workspace .cursor/mcp.json to enable Nano-Android MCP tools in Cursor.",
		}, nil
	case ClientClaudeCode, ClientGenericMCP:
		b, _ := json.MarshalIndent(map[string]any{
			"mcpServers": map[string]any{"nano-android": mcpEntry},
		}, "", "  ")
		return IDEIntegrationBundle{
			Client:        client,
			DisplayName:   "Claude Code / Generic MCP (.mcp.json)",
			ConfigRelPath: ".mcp.json",
			Format:        "json",
			Content:       string(b) + "\n",
			InstallNote:   "Place at repository root .mcp.json for Claude Code and standard MCP hosts.",
		}, nil
	case ClientAntigravity:
		b, _ := json.MarshalIndent(map[string]any{
			"mcpServers": map[string]any{
				"nano-android": map[string]any{
					"command": mcpBinaryPath,
					"args":    args,
					"env":     map[string]string{"NANO_LOG_LEVEL": "INFO"},
				},
			},
		}, "", "  ")
		return IDEIntegrationBundle{
			Client:        client,
			DisplayName:   "Google Antigravity IDE (~/.gemini/antigravity/mcp_config.json)",
			ConfigRelPath: filepath.Join("cache", "ide_configs", "antigravity_mcp_config.json"),
			Format:        "json",
			Content:       string(b) + "\n",
			InstallNote:   "Merge into ~/.gemini/antigravity/mcp_config.json or project .mcp.json.",
		}, nil
	case ClientAndroidStudio, ClientIntelliJIDEA:
		var escBuf bytes.Buffer
		_ = xml.EscapeText(&escBuf, []byte(mcpBinaryPath))
		xmlContent := fmt.Sprintf(`<toolSet name="Nano-Android">
  <tool name="Nano Studio Workstation" description="Launch Nano-Android Studio" showInMainMenu="true" showInEditor="false" showInProject="true" useConsole="true">
    <exec>
      <option name="COMMAND" value="%s" />
      <option name="PARAMETERS" value="doctor --json" />
      <option name="WORKING_DIRECTORY" value="$ProjectFileDir$" />
    </exec>
  </tool>
</toolSet>
`, escBuf.String())
		rel := filepath.Join("cache", "ide_configs", string(client)+"_tools.xml")
		return IDEIntegrationBundle{
			Client:        client,
			DisplayName:   string(client) + " External Tool & MCP Bridge",
			ConfigRelPath: rel,
			Format:        "xml",
			Content:       xmlContent,
			InstallNote:   "Import XML into Settings -> Tools -> External Tools or attach JetBrains AI Assistant MCP server.",
		}, nil
	default:
		return IDEIntegrationBundle{}, fmt.Errorf("unsupported IDE client %q", client)
	}
}

// InstallConfig writes the generated IDE integration config safely inside workspaceRoot/allowedSubdir.
func InstallConfig(client IDEClient, mcpBinaryPath, workspaceRoot string) (string, IDEIntegrationBundle, error) {
	bundle, err := GenerateConfig(client, mcpBinaryPath, workspaceRoot)
	if err != nil {
		return "", bundle, err
	}
	targetDir := filepath.Dir(filepath.Join(workspaceRoot, bundle.ConfigRelPath))
	if err := os.MkdirAll(targetDir, 0750); err != nil {
		return "", bundle, err
	}
	dst, err := pathcontainment.ResolveWriteContained(workspaceRoot, bundle.ConfigRelPath, []string{".json", ".xml"})
	if err != nil {
		return "", bundle, fmt.Errorf("BLOCKED_POLICY %w", err)
	}
	if err := os.WriteFile(dst, []byte(bundle.Content), 0600); err != nil {
		return "", bundle, err
	}
	return dst, bundle, nil
}
