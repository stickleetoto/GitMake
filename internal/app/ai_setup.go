package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gitmake/internal/aiconnect"
	"gitmake/internal/installer"
)

func runAISetup(o Options) error {
	if o.ReadOnly {
		return fmt.Errorf("read-only mode blocks `gitmake ai setup`")
	}
	if o.AIClient == "generic" {
		return runGenericAISetup(o)
	}
	if o.AIWrite && !o.Yes {
		ok, err := confirmAIWriteAccess()
		if err != nil {
			return err
		}
		if !ok {
			if o.JSON {
				return emitJSON(map[string]any{
					"schema":    "gitmake.ai-setup/v1",
					"ok":        false,
					"version":   Version,
					"cancelled": true,
				})
			}
			fmt.Println("Cancelled. Claude Code access was not changed.")
			return nil
		}
	}

	target, err := stableAIExecutable()
	if err != nil {
		return err
	}
	mgr := aiconnect.Manager{GitMakePath: target, Verbose: o.Verbose}
	status, changed, err := mgr.Setup(o.AIWrite)
	if err != nil {
		return err
	}

	if o.JSON {
		return emitJSON(map[string]any{
			"schema":  "gitmake.ai-setup/v1",
			"ok":      true,
			"version": Version,
			"changed": changed,
			"status":  status,
		})
	}

	fmt.Printf("GitMake AI Setup · %s\n\n", Version)
	fmt.Printf("✓ Claude Code       %s\n", firstNonEmpty(status.ClientVersion, "detected"))
	fmt.Printf("✓ GitMake CLI       %s\n", target)
	if changed {
		fmt.Println("✓ MCP registration  configured")
	} else {
		fmt.Println("✓ MCP registration  already configured")
	}
	fmt.Printf("✓ Access            %s\n", status.Access)
	fmt.Printf("✓ Connection        %s\n", friendlyAIHealth(status.Health))
	if strings.TrimSpace(status.HealthDetail) != "" && status.Health != "connected" {
		fmt.Printf("  %s\n", status.HealthDetail)
	}
	fmt.Println("\nReady. Open Claude Code and use GitMake normally.")
	return nil
}

func runAIStatus(o Options) error {
	if o.AIClient == "generic" {
		return runGenericAIStatus(o)
	}
	mgr := aiconnect.Manager{Verbose: o.Verbose}
	status, err := mgr.Status()
	if err != nil {
		return err
	}
	if o.JSON {
		return emitJSON(status)
	}

	fmt.Printf("GitMake AI · %s\n\n", Version)
	if !status.ClientDetected {
		fmt.Println("× Claude Code       not detected")
		fmt.Println("\nInstall Claude Code, then run:\n  gitmake ai setup")
		return nil
	}
	fmt.Printf("✓ Claude Code       %s\n", firstNonEmpty(status.ClientVersion, "detected"))
	if !status.Registered {
		fmt.Println("× MCP registration  not configured")
		fmt.Println("\nRun:\n  gitmake ai setup")
		return nil
	}
	fmt.Println("✓ MCP registration  configured")
	fmt.Printf("✓ Scope             %s\n", firstNonEmpty(status.Scope, "unknown"))
	fmt.Printf("✓ Access            %s\n", status.Access)
	fmt.Printf("✓ Connection        %s\n", friendlyAIHealth(status.Health))
	if status.Command != "" {
		fmt.Printf("· Command           %s %s\n", status.Command, strings.Join(status.Args, " "))
	}
	if strings.TrimSpace(status.HealthDetail) != "" && status.Health != "connected" {
		fmt.Printf("· Detail            %s\n", status.HealthDetail)
	}
	return nil
}

func runAIRemove(o Options) error {
	if o.ReadOnly {
		return fmt.Errorf("read-only mode blocks `gitmake ai remove`")
	}
	if o.AIClient == "generic" {
		if o.JSON {
			return emitJSON(map[string]any{"schema": "gitmake.ai-remove/v1", "ok": true, "client": "generic", "removed": false, "note": "Generic MCP clients are not registered by GitMake; remove the stdio entry in your client."})
		}
		fmt.Println("Generic MCP clients are not registered by GitMake. Remove the stdio entry in your client configuration if needed.")
		return nil
	}
	mgr := aiconnect.Manager{Verbose: o.Verbose}
	removed, status, err := mgr.Remove()
	if err != nil {
		return err
	}
	if o.JSON {
		return emitJSON(map[string]any{
			"schema":  "gitmake.ai-remove/v1",
			"ok":      true,
			"version": Version,
			"removed": removed,
			"status":  status,
		})
	}
	fmt.Printf("GitMake AI Remove · %s\n\n", Version)
	if removed {
		fmt.Println("✓ Claude Code MCP registration removed")
	} else {
		fmt.Println("✓ Nothing to remove")
	}
	return nil
}

func runGenericAISetup(o Options) error {
	target, err := stableAIExecutable()
	if err != nil {
		return err
	}
	args := []string{"mcp"}
	access := "read-only"
	if o.AIWrite {
		if !o.Yes {
			ok, err := confirmAIWriteAccess()
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
		args = append(args, "--allow-write")
		access = "write"
	}
	desc := map[string]any{
		"schema": "gitmake.mcp-registration/v1", "client": "generic", "transport": "stdio",
		"command": target, "args": args, "access": access,
		"note": "Add this stdio server to any MCP-compatible client. GitMake does not modify third-party client config in generic mode.",
	}
	if o.JSON {
		return emitJSON(desc)
	}
	fmt.Printf("GitMake Generic MCP · %s\n\n", Version)
	fmt.Println("Transport          stdio")
	fmt.Println("Command            " + target)
	fmt.Println("Args               " + strings.Join(args, " "))
	fmt.Println("Access             " + access)
	fmt.Println("\nUse these values in any MCP-compatible client.")
	return nil
}

func runGenericAIStatus(o Options) error {
	target, err := stableAIExecutable()
	if err != nil {
		return err
	}
	desc := map[string]any{
		"schema": "gitmake.ai-status/v1", "client": "generic", "server": "gitmake",
		"transport": "stdio", "command": target, "args": []string{"mcp"}, "ready": true,
	}
	if o.JSON {
		return emitJSON(desc)
	}
	fmt.Printf("GitMake Generic MCP · %s\n\n", Version)
	fmt.Println("✓ Server            ready")
	fmt.Println("· Transport         stdio")
	fmt.Println("· Command           " + target + " mcp")
	return nil
}

func stableAIExecutable() (string, error) {
	if runtime.GOOS == "windows" {
		target, _, err := installer.InstallSelf()
		if err != nil {
			return "", fmt.Errorf("prepare GitMake installation for AI setup: %w", err)
		}
		return target, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate GitMake executable: %w", err)
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	return exe, nil
}

func confirmAIWriteAccess() (bool, error) {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false, err
	}
	if st.Mode()&os.ModeCharDevice == 0 {
		return false, fmt.Errorf("non-interactive write setup requires `gitmake ai setup --write --yes`")
	}
	fmt.Println("GitMake AI Setup")
	fmt.Println()
	fmt.Println("AI write access allows:")
	fmt.Println("  • create/update gitmake.json")
	fmt.Println("  • submit reviewed GitMake plans for apply")
	fmt.Println("  • GitHub apply still requires a user-created one-shot approval token")
	fmt.Println()
	fmt.Print("Enable write access? [y/N]: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func friendlyAIHealth(v string) string {
	switch v {
	case "connected":
		return "connected"
	case "configured":
		return "configured"
	case "cached":
		return "cached"
	case "pending_approval":
		return "pending approval"
	case "needs_auth":
		return "needs authentication"
	case "failed":
		return "failed"
	case "client_not_found":
		return "Claude Code not found"
	case "not_configured":
		return "not configured"
	default:
		if strings.TrimSpace(v) == "" {
			return "unknown"
		}
		return v
	}
}
