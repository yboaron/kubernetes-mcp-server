package submariner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initSubmarinerESPRemediation() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "submariner_fix_esp_firewall",
				Description: "Fix IPsec tunnel connectivity issues when ESP protocol is blocked. Forces UDP encapsulation (port 4500) instead of ESP. For ACM/downstream: sets forceUDPEncaps in SubmarinerConfig on each managed cluster. For upstream/subctl: sets ceIPSecForceUDPEncaps in Submariner CR on each cluster.",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"cluster1_kubeconfig": {
							Type:        "string",
							Description: "Path to kubeconfig file for first cluster (Required)",
						},
						"cluster2_kubeconfig": {
							Type:        "string",
							Description: "Path to kubeconfig file for second cluster (Required)",
						},
						"deployment_type": {
							Type:        "string",
							Description: "Deployment type: 'acm' or 'subctl'. Auto-detected if not provided (Optional)",
							Enum:        []any{"acm", "subctl"},
						},
						"namespace": {
							Type:        "string",
							Description: "Namespace where Submariner is installed (Optional, defaults to submariner-operator)",
						},
						"verify_timeout_seconds": {
							Type:        "number",
							Description: "Timeout in seconds to wait for tunnel to come up after fix (Optional, default: 60)",
							Default:     api.ToRawMessage(60),
						},
					},
					Required: []string{"cluster1_kubeconfig", "cluster2_kubeconfig"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Submariner: Fix ESP Firewall Issue",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(false), // Not destructive, but does modify CRs
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: submarinerFixESPFirewall,
		},
	}
}

func submarinerFixESPFirewall(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse arguments
	cluster1Kubeconfig, ok := params.GetArguments()["cluster1_kubeconfig"].(string)
	if !ok || cluster1Kubeconfig == "" {
		return api.NewToolCallResult("", fmt.Errorf("cluster1_kubeconfig is required")), nil
	}

	cluster2Kubeconfig, ok := params.GetArguments()["cluster2_kubeconfig"].(string)
	if !ok || cluster2Kubeconfig == "" {
		return api.NewToolCallResult("", fmt.Errorf("cluster2_kubeconfig is required")), nil
	}

	namespace := defaultSubmarinerNamespace
	if ns := params.GetArguments()["namespace"]; ns != nil {
		namespace = ns.(string)
	}

	deploymentType := ""
	if dt := params.GetArguments()["deployment_type"]; dt != nil {
		deploymentType = dt.(string)
	}

	verifyTimeoutSeconds := 60
	if vts := params.GetArguments()["verify_timeout_seconds"]; vts != nil {
		verifyTimeoutSeconds = int(vts.(float64))
	}

	report := "=== Submariner ESP Firewall Remediation ===\n\n"

	// Step 1: Detect deployment method for both clusters
	report += "Step 1: Detecting deployment method...\n"

	if deploymentType == "" {
		var err error
		deploymentType, err = detectDeploymentMethodKubectl(cluster1Kubeconfig, namespace)
		if err != nil {
			return api.NewToolCallResult(report, fmt.Errorf("failed to detect deployment method for cluster1: %v", err)), nil
		}

		// Verify cluster2 has same deployment method
		deploymentType2, err := detectDeploymentMethodKubectl(cluster2Kubeconfig, namespace)
		if err != nil {
			return api.NewToolCallResult(report, fmt.Errorf("failed to detect deployment method for cluster2: %v", err)), nil
		}

		if deploymentType != deploymentType2 {
			return api.NewToolCallResult(report, fmt.Errorf("clusters have different deployment methods (cluster1: %s, cluster2: %s)", deploymentType, deploymentType2)), nil
		}
	}
	report += fmt.Sprintf("  ✓ Deployment type: %s\n\n", deploymentType)

	// Route to appropriate handler
	if deploymentType == "acm" {
		return handleACMDeployment(report, cluster1Kubeconfig, cluster2Kubeconfig, namespace, verifyTimeoutSeconds)
	}

	return handleSubctlDeployment(report, cluster1Kubeconfig, cluster2Kubeconfig, namespace, verifyTimeoutSeconds)
}

// handleACMDeployment handles ESP remediation for ACM/downstream deployments
// For ACM: set forceUDPEncaps in SubmarinerConfig on each managed cluster
func handleACMDeployment(report, cluster1Kubeconfig, cluster2Kubeconfig, namespace string, verifyTimeoutSeconds int) (*api.ToolCallResult, error) {
	report += "=== ACM/Downstream Deployment Remediation ===\n\n"
	report += "For ACM deployments, forceUDPEncaps must be set in SubmarinerConfig on each managed cluster.\n\n"

	// Step 2: Get current tunnel status
	report += "Step 2: Checking current tunnel status...\n"
	_, err := runSubctlShow(cluster1Kubeconfig)
	if err != nil {
		report += fmt.Sprintf("  ⚠ Warning: Could not get initial tunnel status: %v\n\n", err)
	} else {
		report += "  ✓ Current status captured\n\n"
	}

	// Step 3: Find and patch SubmarinerConfig on cluster1
	report += "Step 3: Finding SubmarinerConfig on Cluster1...\n"
	configName1, configNamespace1, err := findSubmarinerConfig(cluster1Kubeconfig)
	if err != nil {
		return api.NewToolCallResult(report, fmt.Errorf("failed to find SubmarinerConfig on cluster1: %v", err)), nil
	}
	report += fmt.Sprintf("  ✓ Found SubmarinerConfig: %s/%s\n", configNamespace1, configName1)

	report += "  Applying forceUDPEncaps to Cluster1...\n"
	err = patchSubmarinerConfigKubectl(cluster1Kubeconfig, configName1, configNamespace1)
	if err != nil {
		return api.NewToolCallResult(report, fmt.Errorf("failed to patch SubmarinerConfig on cluster1: %v", err)), nil
	}
	report += "  ✓ forceUDPEncaps: true applied to cluster1\n\n"

	// Step 4: Find and patch SubmarinerConfig on cluster2
	report += "Step 4: Finding SubmarinerConfig on Cluster2...\n"
	configName2, configNamespace2, err := findSubmarinerConfig(cluster2Kubeconfig)
	if err != nil {
		return api.NewToolCallResult(report, fmt.Errorf("failed to find SubmarinerConfig on cluster2: %v", err)), nil
	}
	report += fmt.Sprintf("  ✓ Found SubmarinerConfig: %s/%s\n", configNamespace2, configName2)

	report += "  Applying forceUDPEncaps to Cluster2...\n"
	err = patchSubmarinerConfigKubectl(cluster2Kubeconfig, configName2, configNamespace2)
	if err != nil {
		return api.NewToolCallResult(report, fmt.Errorf("failed to patch SubmarinerConfig on cluster2: %v", err)), nil
	}
	report += "  ✓ forceUDPEncaps: true applied to cluster2\n\n"

	// Step 5: Restart gateway pods on cluster1
	report += "Step 5: Restarting gateway pods on Cluster1...\n"
	err = restartGatewayPodsKubectl(cluster1Kubeconfig, namespace)
	if err != nil {
		report += fmt.Sprintf("  ⚠ Warning: Failed to restart gateway pods on cluster1: %v\n", err)
		report += "  Note: Gateway pods may restart automatically based on SubmarinerConfig changes\n\n"
	} else {
		report += "  ✓ Gateway pods restarted on cluster1\n\n"
	}

	// Step 6: Restart gateway pods on cluster2
	report += "Step 6: Restarting gateway pods on Cluster2...\n"
	err = restartGatewayPodsKubectl(cluster2Kubeconfig, namespace)
	if err != nil {
		report += fmt.Sprintf("  ⚠ Warning: Failed to restart gateway pods on cluster2: %v\n", err)
		report += "  Note: Gateway pods may restart automatically based on SubmarinerConfig changes\n\n"
	} else {
		report += "  ✓ Gateway pods restarted on cluster2\n\n"
	}

	// Step 7: Wait and verify tunnel status
	report += fmt.Sprintf("Step 7: Waiting up to %d seconds for tunnel to come up...\n", verifyTimeoutSeconds)
	connected, finalStatus := waitForTunnelConnection(cluster1Kubeconfig, verifyTimeoutSeconds)

	if connected {
		report += "  ✓ SUCCESS: Tunnel is now connected!\n\n"
		report += "=== Final Tunnel Status ===\n"
		report += finalStatus + "\n"
		report += "\nThe ESP firewall issue has been resolved. The IPSec tunnel is now using UDP encapsulation (port 4500) instead of ESP protocol.\n"
	} else {
		report += "  ⚠ Tunnel did not come up within the timeout period\n\n"
		report += "=== Current Tunnel Status ===\n"
		report += finalStatus + "\n"
		report += "\nThe fix has been applied, but the tunnel is not yet connected. Possible reasons:\n"
		report += "  - The issue may not be related to ESP protocol\n"
		report += "  - Additional firewall rules may be blocking UDP port 4500\n"
		report += "  - The gateway pods may need more time to establish the connection\n"
		report += "\nPlease run 'submariner_health_check' again to verify the status.\n"
	}

	return api.NewToolCallResult(report, nil), nil
}

// handleSubctlDeployment handles ESP remediation for subctl deployments
func handleSubctlDeployment(report, cluster1Kubeconfig, cluster2Kubeconfig, namespace string, verifyTimeoutSeconds int) (*api.ToolCallResult, error) {
	if cluster2Kubeconfig == "" {
		return api.NewToolCallResult(report, fmt.Errorf("cluster2_kubeconfig is required for subctl deployments")), nil
	}

	report += "=== Subctl Deployment Remediation ===\n\n"

	// Step 2: Verify both clusters are subctl deployments
	report += "Step 2: Verifying both clusters are subctl deployments...\n"

	deploymentMethod2, err := detectDeploymentMethodKubectl(cluster2Kubeconfig, namespace)
	if err != nil {
		return api.NewToolCallResult(report, fmt.Errorf("failed to detect deployment method for cluster2: %v", err)), nil
	}
	if deploymentMethod2 != "subctl" {
		return api.NewToolCallResult(report, fmt.Errorf("cluster2 is not a subctl deployment (detected: %s)", deploymentMethod2)), nil
	}
	report += "  ✓ Both clusters are subctl deployments\n\n"

	// Step 3: Get current tunnel status
	report += "Step 3: Checking current tunnel status...\n"
	_, err = runSubctlShow(cluster1Kubeconfig)
	if err != nil {
		report += fmt.Sprintf("  ⚠ Warning: Could not get initial tunnel status: %v\n\n", err)
	} else {
		report += "  ✓ Current status captured\n\n"
	}

	// Step 4: Apply ceIPSecForceUDPEncaps to cluster1
	report += "Step 4: Applying ceIPSecForceUDPEncaps to Cluster1...\n"
	err = patchSubmarinerCRKubectl(cluster1Kubeconfig, namespace)
	if err != nil {
		return api.NewToolCallResult(report, fmt.Errorf("failed to patch Submariner CR on cluster1: %v", err)), nil
	}
	report += "  ✓ ceIPSecForceUDPEncaps: true applied to cluster1\n\n"

	// Step 5: Apply ceIPSecForceUDPEncaps to cluster2
	report += "Step 5: Applying ceIPSecForceUDPEncaps to Cluster2...\n"
	err = patchSubmarinerCRKubectl(cluster2Kubeconfig, namespace)
	if err != nil {
		return api.NewToolCallResult(report, fmt.Errorf("failed to patch Submariner CR on cluster2: %v", err)), nil
	}
	report += "  ✓ ceIPSecForceUDPEncaps: true applied to cluster2\n\n"

	// Step 6: Restart gateway pods on cluster1
	report += "Step 6: Restarting gateway pods on Cluster1...\n"
	err = restartGatewayPodsKubectl(cluster1Kubeconfig, namespace)
	if err != nil {
		return api.NewToolCallResult(report, fmt.Errorf("failed to restart gateway pods on cluster1: %v", err)), nil
	}
	report += "  ✓ Gateway pods restarted on cluster1\n\n"

	// Step 7: Restart gateway pods on cluster2
	report += "Step 7: Restarting gateway pods on Cluster2...\n"
	err = restartGatewayPodsKubectl(cluster2Kubeconfig, namespace)
	if err != nil {
		return api.NewToolCallResult(report, fmt.Errorf("failed to restart gateway pods on cluster2: %v", err)), nil
	}
	report += "  ✓ Gateway pods restarted on cluster2\n\n"

	// Step 8: Wait and verify tunnel status
	report += fmt.Sprintf("Step 8: Waiting up to %d seconds for tunnel to come up...\n", verifyTimeoutSeconds)
	connected, finalStatus := waitForTunnelConnection(cluster1Kubeconfig, verifyTimeoutSeconds)

	if connected {
		report += "  ✓ SUCCESS: Tunnel is now connected!\n\n"
		report += "=== Final Tunnel Status ===\n"
		report += finalStatus + "\n"
		report += "\nThe ESP firewall issue has been resolved. The IPSec tunnel is now using UDP encapsulation (port 4500) instead of ESP protocol.\n"
	} else {
		report += "  ⚠ Tunnel did not come up within the timeout period\n\n"
		report += "=== Current Tunnel Status ===\n"
		report += finalStatus + "\n"
		report += "\nThe fix has been applied, but the tunnel is not yet connected. Possible reasons:\n"
		report += "  - The issue may not be related to ESP protocol\n"
		report += "  - Additional firewall rules may be blocking UDP port 4500\n"
		report += "  - The gateway pods may need more time to establish the connection\n"
		report += "\nPlease run 'submariner_health_check' again to verify the status.\n"
	}

	return api.NewToolCallResult(report, nil), nil
}

// detectDeploymentMethodKubectl detects deployment method using kubectl
func detectDeploymentMethodKubectl(kubeconfigPath, namespace string) (string, error) {
	args := []string{"get", "pods", "-n", namespace, "--kubeconfig", kubeconfigPath, "-o", "name"}
	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown", fmt.Errorf("failed to list pods: %v", err)
	}

	// Check if submariner-addon pod exists
	if strings.Contains(string(output), "submariner-addon") {
		return "acm", nil
	}

	return "subctl", nil
}

// patchSubmarinerCRKubectl patches the Submariner CR to add ceIPSecForceUDPEncaps: true
func patchSubmarinerCRKubectl(kubeconfigPath, namespace string) error {
	// First check if ceIPSecForceUDPEncaps is already set
	args := []string{"get", "submariner", "submariner", "-n", namespace,
		"--kubeconfig", kubeconfigPath, "-o", "jsonpath={.spec.ceIPSecForceUDPEncaps}"}
	cmd := exec.Command("kubectl", args...)
	output, _ := cmd.CombinedOutput()

	if strings.TrimSpace(string(output)) == "true" {
		return nil // Already set
	}

	// Patch the CR
	patchArgs := []string{"patch", "submariner", "submariner", "-n", namespace,
		"--kubeconfig", kubeconfigPath, "--type=merge",
		"-p", `{"spec":{"ceIPSecForceUDPEncaps":true}}`}
	patchCmd := exec.Command("kubectl", patchArgs...)
	patchOutput, err := patchCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to patch Submariner CR: %v\nOutput: %s", err, string(patchOutput))
	}

	return nil
}

// restartGatewayPodsKubectl deletes the gateway pods to trigger a restart
func restartGatewayPodsKubectl(kubeconfigPath, namespace string) error {
	args := []string{"delete", "pods", "-n", namespace, "-l", "app=submariner-gateway",
		"--kubeconfig", kubeconfigPath}
	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete gateway pods: %v\nOutput: %s", err, string(output))
	}

	return nil
}

// waitForTunnelConnection waits for the tunnel to come up and returns status
func waitForTunnelConnection(kubeconfigPath string, timeoutSeconds int) (bool, string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	timeout := time.After(time.Duration(timeoutSeconds) * time.Second)

	for {
		select {
		case <-timeout:
			// Timeout reached, get final status
			status, _ := runSubctlShow(kubeconfigPath)
			return false, status

		case <-ticker.C:
			// Check tunnel status
			status, err := runSubctlShow(kubeconfigPath)
			if err != nil {
				continue
			}

			// Parse status to check if connected
			if strings.Contains(status, "connected") {
				// Verify it's in the STATUS column
				lines := strings.Split(status, "\n")
				for _, line := range lines {
					if strings.Contains(line, "connected") && !strings.HasPrefix(strings.TrimSpace(line), "GATEWAY") {
						return true, status
					}
				}
			}
		}
	}
}

// findSubmarinerConfig finds the SubmarinerConfig resource on a managed cluster
func findSubmarinerConfig(kubeconfigPath string) (string, string, error) {
	// For ACM deployments, SubmarinerConfig is typically in the managed cluster's submariner-operator namespace
	// Try common namespaces
	commonNamespaces := []string{
		"submariner-operator",
		"open-cluster-management-agent-addon",
		"default",
	}

	for _, ns := range commonNamespaces {
		args := []string{"get", "submarinerconfig", "-n", ns, "--kubeconfig", kubeconfigPath, "-o", "name"}
		cmd := exec.Command("kubectl", args...)
		output, err := cmd.CombinedOutput()
		if err == nil && len(output) > 0 {
			// Found SubmarinerConfig in this namespace
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			if len(lines) > 0 {
				// Extract name from "submarinerconfig.submarineraddon.open-cluster-management.io/name" format
				parts := strings.Split(lines[0], "/")
				if len(parts) == 2 {
					return parts[1], ns, nil
				}
				// Handle case where only name is returned
				return strings.TrimSpace(lines[0]), ns, nil
			}
		}
	}

	// Try to search in all namespaces
	args := []string{"get", "submarinerconfig", "--all-namespaces", "--kubeconfig", kubeconfigPath, "-o", "jsonpath={.items[0].metadata.name},{.items[0].metadata.namespace}"}
	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("no SubmarinerConfig resources found: %v", err)
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "," {
		return "", "", fmt.Errorf("no SubmarinerConfig resources found")
	}

	parts := strings.Split(outputStr, ",")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected output format when searching for SubmarinerConfig: %s", outputStr)
	}

	return parts[0], parts[1], nil
}

// patchSubmarinerConfigKubectl patches the SubmarinerConfig CR to add forceUDPEncaps: true
func patchSubmarinerConfigKubectl(kubeconfigPath, configName, configNamespace string) error {
	// First check if forceUDPEncaps is already set
	args := []string{"get", "submarinerconfig", configName, "-n", configNamespace,
		"--kubeconfig", kubeconfigPath, "-o", "jsonpath={.spec.forceUDPEncaps}"}
	cmd := exec.Command("kubectl", args...)
	output, _ := cmd.CombinedOutput()

	if strings.TrimSpace(string(output)) == "true" {
		return nil // Already set
	}

	// Patch the SubmarinerConfig
	patchArgs := []string{"patch", "submarinerconfig", configName, "-n", configNamespace,
		"--kubeconfig", kubeconfigPath, "--type=merge",
		"-p", `{"spec":{"forceUDPEncaps":true}}`}
	patchCmd := exec.Command("kubectl", patchArgs...)
	patchOutput, err := patchCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to patch SubmarinerConfig: %v\nOutput: %s", err, string(patchOutput))
	}

	return nil
}
