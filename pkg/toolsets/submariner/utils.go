package submariner

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	subctlInstallScriptURL = "https://get.submariner.io"
	subctlVersion          = "latest" // Can be "latest", "devel", "rc", or specific version like "0.21.0"
)

// findOrInstallSubctl locates the subctl binary, installing it if not found
func findOrInstallSubctl() (string, error) {
	// First try to find it in PATH
	path, err := exec.LookPath("subctl")
	if err == nil {
		return path, nil
	}

	// Try common installation locations
	homeDir := os.Getenv("HOME")
	commonPaths := []string{
		filepath.Join(homeDir, ".local", "bin", "subctl"), // Default install location
		"/usr/local/bin/subctl",
		"/usr/bin/subctl",
		filepath.Join(homeDir, "bin", "subctl"),
		filepath.Join(os.Getenv("GOPATH"), "bin", "subctl"),
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// subctl not found, attempt to install it
	fmt.Println("subctl binary not found, attempting automatic installation...")
	if err := installSubctl(); err != nil {
		return "", fmt.Errorf("subctl binary not found and automatic installation failed: %v\nPlease install subctl manually from https://submariner.io/operations/deployment/subctl/", err)
	}

	// After installation, check the default location
	defaultPath := filepath.Join(homeDir, ".local", "bin", "subctl")
	if _, err := os.Stat(defaultPath); err == nil {
		fmt.Printf("✓ subctl successfully installed to %s\n", defaultPath)
		return defaultPath, nil
	}

	return "", fmt.Errorf("subctl installation appeared to succeed but binary not found at expected location: %s", defaultPath)
}

// installSubctl downloads and installs subctl using the official installation script
func installSubctl() error {
	// Only support Linux and macOS for automatic installation
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic installation only supported on Linux and macOS, detected OS: %s", runtime.GOOS)
	}

	// Download the installation script
	resp, err := http.Get(subctlInstallScriptURL)
	if err != nil {
		return fmt.Errorf("failed to download installation script: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download installation script: HTTP %d", resp.StatusCode)
	}

	// Read the script content
	scriptContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read installation script: %v", err)
	}

	// Create a temporary file for the script
	tmpFile, err := os.CreateTemp("", "subctl-install-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write the script to the temporary file
	if _, err := tmpFile.Write(scriptContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write installation script: %v", err)
	}
	tmpFile.Close()

	// Make the script executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		return fmt.Errorf("failed to make script executable: %v", err)
	}

	// Execute the installation script
	cmd := exec.Command("/bin/bash", tmpFile.Name())
	cmd.Env = append(os.Environ(), fmt.Sprintf("VERSION=%s", subctlVersion))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installation script failed: %v", err)
	}

	return nil
}

// runSubctlFirewallInterCluster executes 'subctl diagnose firewall inter-cluster' command
func runSubctlFirewallInterCluster(fromKubeconfig, toKubeconfig, fromContext, toContext string, verbose bool) (string, error) {
	subctlPath, err := findSubctl()
	if err != nil {
		return "", err
	}

	args := []string{"diagnose", "firewall", "inter-cluster"}

	// Add kubeconfig for source cluster
	if fromContext != "" {
		args = append(args, "--context", fromContext)
	}
	args = append(args, "--kubeconfig", fromKubeconfig)

	// Add kubeconfig for destination cluster
	if toContext != "" {
		args = append(args, "--remotecontext", toContext)
	}
	args = append(args, "--remoteconfig", toKubeconfig)

	if verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.Command(subctlPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Include output even on error as it may contain useful diagnostic info
		return string(output), fmt.Errorf("subctl firewall diagnostic failed: %v", err)
	}

	return string(output), nil
}

// parseSubctlOutput extracts key information from subctl output
func parseSubctlOutput(output string) map[string]interface{} {
	result := make(map[string]interface{})

	// Count checkmarks and errors
	result["checks_passed"] = strings.Count(output, "✓")
	result["checks_failed"] = strings.Count(output, "✗")
	result["warnings"] = strings.Count(output, "⚠")

	// Check for connection status
	if strings.Contains(output, "connected") {
		result["has_connections"] = true
	}

	// Check for errors
	if strings.Contains(output, "error") || strings.Contains(output, "Error") || strings.Contains(output, "ERROR") {
		result["has_errors"] = true
	}

	return result
}
