package submariner

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

const (
	defaultSubmarinerNamespace = "submariner-operator"
	defaultLogTailLines        = 100
)

func initSubmarinerHealth() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "submariner_health_check",
				Description: "Comprehensive health check for Submariner deployment including pod status, log analysis, and subctl diagnostics",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace where Submariner is installed (Optional, defaults to submariner-operator)",
						},
						"kubeconfig_path": {
							Type:        "string",
							Description: "Path to the kubeconfig file for the cluster (Optional, uses default kubeconfig if not provided)",
						},
						"include_subctl_show": {
							Type:        "boolean",
							Description: "Include output from 'subctl show all' command (Optional, default: true)",
							Default:     api.ToRawMessage(true),
						},
						"include_subctl_diagnose": {
							Type:        "boolean",
							Description: "Include output from 'subctl diagnose all' command (Optional, default: true)",
							Default:     api.ToRawMessage(true),
						},
						"check_logs": {
							Type:        "boolean",
							Description: "Analyze pod logs for errors (Optional, default: true)",
							Default:     api.ToRawMessage(true),
						},
					},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Submariner: Health Check",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: submarinerHealthCheck,
		},
	}
}

func submarinerHealthCheck(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse arguments
	namespace := defaultSubmarinerNamespace
	if ns := params.GetArguments()["namespace"]; ns != nil {
		namespace = ns.(string)
	}

	kubeconfigPath := ""
	if kcp := params.GetArguments()["kubeconfig_path"]; kcp != nil {
		kubeconfigPath = kcp.(string)
	}

	includeSubctlShow := true
	if iss := params.GetArguments()["include_subctl_show"]; iss != nil {
		includeSubctlShow = iss.(bool)
	}

	includeSubctlDiagnose := true
	if isd := params.GetArguments()["include_subctl_diagnose"]; isd != nil {
		includeSubctlDiagnose = isd.(bool)
	}

	checkLogs := true
	if cl := params.GetArguments()["check_logs"]; cl != nil {
		checkLogs = cl.(bool)
	}

	// Build health report
	report := "=== Submariner Health Check Report ===\n"
	report += fmt.Sprintf("Namespace: %s\n\n", namespace)

	// 1. Check pod status
	podReport, err := checkSubmarinerPods(params, namespace)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to check Submariner pods: %v", err)), nil
	}
	report += podReport + "\n"

	// 2. Analyze pod logs for errors
	if checkLogs {
		logReport, err := analyzeSubmarinerLogs(params, namespace)
		if err != nil {
			report += fmt.Sprintf("⚠ Warning: Failed to analyze pod logs: %v\n\n", err)
		} else {
			report += logReport + "\n"
		}
	}

	// 3. Run subctl show all
	var subctlShowOutput string
	if includeSubctlShow {
		var err error
		subctlShowOutput, err = runSubctlShow(kubeconfigPath)
		if err != nil {
			report += fmt.Sprintf("⚠ Warning: Failed to run 'subctl show all': %v\n\n", err)
		} else {
			report += "=== Subctl Show All ===\n"
			report += subctlShowOutput + "\n"
		}
	}

	// 4. Detect deployment method and analyze ESP/Firewall issues using Gateway CR
	deploymentMethod := detectDeploymentMethod(params, namespace)
	espAnalysis := analyzeESPFirewallIssuesFromGateway(params, namespace, deploymentMethod)
	if espAnalysis != "" {
		report += espAnalysis + "\n"
	}

	// 5. Run subctl diagnose all
	if includeSubctlDiagnose {
		subctlDiagnoseOutput, err := runSubctlDiagnose(kubeconfigPath)
		if err != nil {
			report += fmt.Sprintf("⚠ Warning: Failed to run 'subctl diagnose all': %v\n\n", err)
		} else {
			report += "=== Subctl Diagnose All ===\n"
			report += subctlDiagnoseOutput + "\n"
		}
	}

	return api.NewToolCallResult(report, nil), nil
}

// checkSubmarinerPods verifies that all Submariner pods are running
func checkSubmarinerPods(params api.ToolHandlerParams, namespace string) (string, error) {
	report := "=== Pod Status ===\n"

	// List all pods in the Submariner namespace
	podListRaw, err := params.PodsListInNamespace(params, namespace, kubernetes.ResourceListOptions{
		AsTable: false,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list pods in namespace %s: %v", namespace, err)
	}

	// Convert runtime.Unstructured to *unstructured.UnstructuredList
	podListUnstructured, ok := podListRaw.(*unstructured.UnstructuredList)
	if !ok {
		return "", fmt.Errorf("failed to convert pod list to unstructured list")
	}

	if len(podListUnstructured.Items) == 0 {
		return report + "⚠ No Submariner pods found in namespace " + namespace + "\n", nil
	}

	allHealthy := true
	var unhealthyPods []string

	for _, podItem := range podListUnstructured.Items {
		pod := &corev1.Pod{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podItem.Object, pod); err != nil {
			continue
		}

		status := "✓"
		if pod.Status.Phase != corev1.PodRunning {
			status = "✗"
			allHealthy = false
			unhealthyPods = append(unhealthyPods, pod.Name)
		}

		// Check container statuses
		containerStatus := ""
		readyContainers := 0
		totalContainers := len(pod.Status.ContainerStatuses)

		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready {
				readyContainers++
			} else {
				allHealthy = false
				if status == "✓" {
					status = "⚠"
				}
			}
		}

		if totalContainers > 0 {
			containerStatus = fmt.Sprintf(" (%d/%d ready)", readyContainers, totalContainers)
		}

		report += fmt.Sprintf("%s %s: %s%s\n", status, pod.Name, pod.Status.Phase, containerStatus)
	}

	if allHealthy {
		report += fmt.Sprintf("\n✓ All %d Submariner pods are healthy\n", len(podListUnstructured.Items))
	} else {
		report += fmt.Sprintf("\n✗ %d pods are not healthy: %s\n", len(unhealthyPods), strings.Join(unhealthyPods, ", "))
	}

	return report, nil
}

// analyzeSubmarinerLogs analyzes pod logs for common error patterns
func analyzeSubmarinerLogs(params api.ToolHandlerParams, namespace string) (string, error) {
	report := "=== Log Analysis ===\n"

	// List all pods in the Submariner namespace
	podListRaw, err := params.PodsListInNamespace(params, namespace, kubernetes.ResourceListOptions{
		AsTable: false,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list pods: %v", err)
	}

	// Convert runtime.Unstructured to *unstructured.UnstructuredList
	podListUnstructured, ok := podListRaw.(*unstructured.UnstructuredList)
	if !ok {
		return "", fmt.Errorf("failed to convert pod list to unstructured list")
	}

	errorPatterns := []string{
		"error",
		"Error",
		"ERROR",
		"failed",
		"Failed",
		"FAILED",
		"panic",
		"fatal",
		"Fatal",
		"FATAL",
	}

	totalErrors := 0
	podErrorCounts := make(map[string]int)

	for _, podItem := range podListUnstructured.Items {
		pod := &corev1.Pod{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podItem.Object, pod); err != nil {
			continue
		}

		// Get logs for each pod
		logs, err := params.PodsLog(params.Context, namespace, pod.Name, "", false, defaultLogTailLines)
		if err != nil {
			report += fmt.Sprintf("⚠ Could not retrieve logs for %s: %v\n", pod.Name, err)
			continue
		}

		// Count errors in logs
		errorCount := 0
		logLines := strings.Split(logs, "\n")
		var errorLines []string

		for _, line := range logLines {
			for _, pattern := range errorPatterns {
				if strings.Contains(line, pattern) {
					errorCount++
					if len(errorLines) < 5 { // Keep only first 5 error lines per pod
						errorLines = append(errorLines, strings.TrimSpace(line))
					}
					break
				}
			}
		}

		if errorCount > 0 {
			totalErrors += errorCount
			podErrorCounts[pod.Name] = errorCount
			report += fmt.Sprintf("\n⚠ %s: Found %d error(s) in logs (last %d lines)\n", pod.Name, errorCount, defaultLogTailLines)
			if len(errorLines) > 0 {
				report += "  Sample errors:\n"
				for _, errLine := range errorLines {
					report += fmt.Sprintf("    - %s\n", errLine)
				}
			}
		}
	}

	if totalErrors == 0 {
		report += "✓ No errors found in pod logs (last 100 lines checked per pod)\n"
	} else {
		report += fmt.Sprintf("\n⚠ Total errors found: %d across %d pod(s)\n", totalErrors, len(podErrorCounts))
	}

	return report, nil
}

// runSubctlShow executes 'subctl show all' command
func runSubctlShow(kubeconfigPath string) (string, error) {
	subctlPath, err := findSubctl()
	if err != nil {
		return "", err
	}

	args := []string{"show", "all"}
	if kubeconfigPath != "" {
		args = append(args, "--kubeconfig", kubeconfigPath)
	}

	cmd := exec.Command(subctlPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("subctl command failed: %v\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// runSubctlDiagnose executes 'subctl diagnose all' command
func runSubctlDiagnose(kubeconfigPath string) (string, error) {
	subctlPath, err := findSubctl()
	if err != nil {
		return "", err
	}

	args := []string{"diagnose", "all"}
	if kubeconfigPath != "" {
		args = append(args, "--kubeconfig", kubeconfigPath)
	}

	cmd := exec.Command(subctlPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("subctl command failed: %v\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// findSubctl locates the subctl binary in the system PATH, installing it if not found
func findSubctl() (string, error) {
	return findOrInstallSubctl()
}

// detectDeploymentMethod checks if Submariner was deployed via subctl or ACM
// Returns "subctl" if submariner-addon pod is missing, "acm" if present, "unknown" on error
func detectDeploymentMethod(params api.ToolHandlerParams, namespace string) string {
	// List all pods in the Submariner namespace
	podListRaw, err := params.PodsListInNamespace(params, namespace, kubernetes.ResourceListOptions{
		AsTable: false,
	})
	if err != nil {
		return "unknown"
	}

	podListUnstructured, ok := podListRaw.(*unstructured.UnstructuredList)
	if !ok {
		return "unknown"
	}

	// Check if submariner-addon pod exists
	for _, podItem := range podListUnstructured.Items {
		pod := &corev1.Pod{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podItem.Object, pod); err != nil {
			continue
		}
		if strings.HasPrefix(pod.Name, "submariner-addon") {
			return "acm"
		}
	}

	// If submariner-addon is missing, it's a subctl deployment
	return "subctl"
}

// GatewayConnection represents a connection in the Gateway CR status
type GatewayConnection struct {
	ClusterID  string
	UsingIP    string
	PrivateIP  string
	PublicIP   string
	Status     string
	GatewayNode string
}

// analyzeESPFirewallIssuesFromGateway checks Gateway CR for ESP firewall issues
// by examining if connections are using private IP and not connected
func analyzeESPFirewallIssuesFromGateway(params api.ToolHandlerParams, namespace string, deploymentMethod string) string {
	// Get all Gateway resources
	gvk := &schema.GroupVersionKind{
		Group:   "submariner.io",
		Version: "v1",
		Kind:    "Gateway",
	}

	gatewaysRaw, err := params.ResourcesList(params.Context, gvk, namespace, kubernetes.ResourceListOptions{
		AsTable: false,
	})
	if err != nil {
		return ""
	}

	gatewaysList, ok := gatewaysRaw.(*unstructured.UnstructuredList)
	if !ok || len(gatewaysList.Items) == 0 {
		return ""
	}

	// Find the active gateway and analyze connections
	var gatewayIssues []GatewayConnection
	var activeGatewayName string

	for _, gatewayItem := range gatewaysList.Items {
		// Check if this is the active gateway
		haStatus, found, err := unstructured.NestedString(gatewayItem.Object, "status", "haStatus")
		if err != nil || !found || haStatus != "active" {
			continue
		}

		activeGatewayName = gatewayItem.GetName()

		// Get connections array
		connections, found, err := unstructured.NestedSlice(gatewayItem.Object, "status", "connections")
		if err != nil || !found {
			continue
		}

		// Analyze each connection
		for _, conn := range connections {
			connMap, ok := conn.(map[string]interface{})
			if !ok {
				continue
			}

			// Get connection status
			status, _, _ := unstructured.NestedString(connMap, "status")
			usingIP, _, _ := unstructured.NestedString(connMap, "usingIP")

			// Get endpoint details
			endpoint, found, _ := unstructured.NestedMap(connMap, "endpoint")
			if !found {
				continue
			}

			clusterID, _, _ := unstructured.NestedString(endpoint, "cluster_id")
			privateIP, _, _ := unstructured.NestedString(endpoint, "private_ip")
			publicIP, _, _ := unstructured.NestedString(endpoint, "public_ip")

			// Check if using private IP and not connected
			if usingIP == privateIP && usingIP != publicIP && status != "connected" {
				gatewayIssues = append(gatewayIssues, GatewayConnection{
					ClusterID:   clusterID,
					UsingIP:     usingIP,
					PrivateIP:   privateIP,
					PublicIP:    publicIP,
					Status:      status,
					GatewayNode: activeGatewayName,
				})
			}
		}
	}

	// If no issues found, return success message
	if len(gatewayIssues) == 0 {
		report := "=== ESP/Firewall Analysis ===\n"
		report += "✓ No ESP firewall issues detected\n"
		report += "  All connections are either established or using public IP (UDP encapsulation)\n"
		return report
	}

	// Build issue report
	report := "=== ESP/Firewall Analysis ===\n"
	report += "⚠ POTENTIAL ESP FIREWALL ISSUE DETECTED\n\n"

	if deploymentMethod != "unknown" {
		report += fmt.Sprintf("Deployment Method Detected: %s\n\n", strings.ToUpper(deploymentMethod))
	}

	report += fmt.Sprintf("Active Gateway: %s\n\n", activeGatewayName)
	report += "The following connection(s) are not connected and using private IP (ESP protocol):\n"
	for _, issue := range gatewayIssues {
		report += fmt.Sprintf("  • Cluster '%s': Status=%s, Using IP=%s (private), Public IP=%s\n",
			issue.ClusterID, issue.Status, issue.UsingIP, issue.PublicIP)
	}

	report += "\n"
	report += "Root Cause Analysis:\n"
	report += "  When the gateway selects private IP (usingIP = private_ip), IPSec traffic is\n"
	report += "  encapsulated in ESP (IP protocol 50) instead of UDP port 4500.\n"
	report += "  If ESP protocol is not allowed in your firewall, the tunnel cannot be established.\n"
	report += "\n"
	report += "Recommended Actions:\n"
	report += "  1. Verify if ESP protocol (IP protocol 50) is blocked in your firewall by checking:\n"
	report += "     - Cloud provider security groups\n"
	report += "     - On-premise firewall rules\n"
	report += "     - iptables/nftables rules on the nodes\n"
	report += "\n"
	report += "  2. To confirm this is the issue, force UDP encapsulation on BOTH clusters:\n"
	report += "\n"
	report += "     IMPORTANT: This setting must be applied to ALL clusters in your ClusterSet!\n"
	report += "\n"

	// Provide deployment-specific instructions
	if deploymentMethod == "subctl" {
		report += "     Your deployment method: SUBCTL\n"
		report += "     \n"
		report += "     You can use the 'submariner_fix_esp_firewall' tool to automatically apply this fix,\n"
		report += "     or manually edit the Submariner CR on each cluster:\n"
		report += "     \n"
		report += "       kubectl patch submariner submariner -n submariner-operator --kubeconfig <cluster-kubeconfig> --type=merge -p '{\"spec\":{\"ceIPSecForceUDPEncaps\":true}}'\n"
		report += "     \n"
		report += "     Or manually edit:\n"
		report += "     \n"
		report += "       kubectl edit submariner submariner -n submariner-operator --kubeconfig <cluster-kubeconfig>\n"
		report += "     \n"
		report += "     Add the following to the spec section on EACH cluster:\n"
		report += "     \n"
		report += "       spec:\n"
		report += "         ceIPSecForceUDPEncaps: true\n"
		report += "     \n"
		report += "     After editing, restart the gateway pods on EACH cluster:\n"
		report += "     \n"
		report += "       kubectl delete pods -n submariner-operator -l app=submariner-gateway --kubeconfig <cluster-kubeconfig>\n"
		report += "\n"
	} else if deploymentMethod == "acm" {
		report += "     Your deployment method: RedHat ACM\n"
		report += "     \n"
		report += "     You can use the 'submariner_fix_esp_firewall' tool to automatically apply this fix,\n"
		report += "     or manually edit the SubmarinerConfig resource on EACH managed cluster:\n"
		report += "     \n"
		report += "       kubectl edit submarinerconfig <config-name> -n submariner-operator --kubeconfig <cluster-kubeconfig>\n"
		report += "     \n"
		report += "     Add the following to the spec section on EACH managed cluster:\n"
		report += "     \n"
		report += "       spec:\n"
		report += "         forceUDPEncaps: true\n"
		report += "     \n"
		report += "     After editing, restart the gateway pods on EACH managed cluster:\n"
		report += "     \n"
		report += "       kubectl delete pods -n submariner-operator -l app=submariner-gateway --kubeconfig <cluster-kubeconfig>\n"
		report += "\n"
	} else {
		report += "     Choose the appropriate method based on your deployment:\n"
		report += "     \n"
		report += "     Option A - If deployed using subctl:\n"
		report += "       kubectl patch submariner submariner -n submariner-operator --kubeconfig <cluster-kubeconfig> --type=merge -p '{\"spec\":{\"ceIPSecForceUDPEncaps\":true}}'\n"
		report += "       Then restart gateway pods: kubectl delete pods -n submariner-operator -l app=submariner-gateway --kubeconfig <cluster-kubeconfig>\n"
		report += "     \n"
		report += "     Option B - If deployed using RedHat ACM:\n"
		report += "       kubectl edit submarinerconfig <config-name> -n <namespace>\n"
		report += "       spec:\n"
		report += "         ceIPSecForceUDPEncaps: true\n"
		report += "\n"
	}

	report += "  3. If the tunnel comes up after setting ceIPSecForceUDPEncaps: true on both clusters:\n"
	report += "     - Keep this setting (recommended if you cannot allow ESP in firewall)\n"
	report += "     - Or allow ESP protocol (IP protocol 50) in your firewall and remove the setting\n"
	report += "\n"

	return report
}

// analyzeESPFirewallIssues checks for ESP protocol firewall issues based on NAT configuration and connection status
// This function is deprecated in favor of analyzeESPFirewallIssuesFromGateway which uses Gateway CR
func analyzeESPFirewallIssues(subctlShowOutput string, deploymentMethod string) string {
	if subctlShowOutput == "" {
		return ""
	}

	report := "=== ESP/Firewall Analysis ===\n"

	lines := strings.Split(subctlShowOutput, "\n")
	inGatewaySection := false
	foundIssues := false
	var gatewayIssues []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Detect the GATEWAY section header
		if strings.HasPrefix(line, "GATEWAY") && strings.Contains(line, "CLUSTER") {
			inGatewaySection = true
			continue
		}

		// Exit gateway section when we hit an empty line or new section
		if inGatewaySection && (line == "" || (strings.HasPrefix(line, "CLUSTER") && strings.Contains(line, "ENDPOINT IP"))) {
			inGatewaySection = false
			continue
		}

		// Parse gateway lines
		if inGatewaySection && line != "" {
			fields := strings.Fields(line)

			// Expected format: GATEWAY CLUSTER REMOTE_IP NAT CABLE_DRIVER SUBNETS STATUS RTT
			// Minimum fields: gateway, cluster, remote_ip, nat, cable_driver, subnets, status
			if len(fields) >= 7 {
				gatewayName := fields[0]
				clusterName := fields[1]
				natStatus := fields[3]
				status := fields[len(fields)-2] // Status is second to last (before RTT)

				// Check if NAT is disabled (private IP used = ESP protocol) and connection is not established
				if strings.ToLower(natStatus) == "no" && strings.ToLower(status) != "connected" {
					foundIssues = true
					issue := fmt.Sprintf("Gateway '%s' to cluster '%s': Status=%s, NAT=%s (ESP protocol in use)",
						gatewayName, clusterName, status, natStatus)
					gatewayIssues = append(gatewayIssues, issue)
				}
			}
		}
	}

	if !foundIssues {
		report += "✓ No ESP firewall issues detected\n"
		report += "  All gateways are either connected or using UDP encapsulation (NAT=yes)\n"
		return report
	}

	// Report issues found
	report += "⚠ POTENTIAL ESP FIREWALL ISSUE DETECTED\n\n"

	if deploymentMethod != "unknown" {
		report += fmt.Sprintf("Deployment Method Detected: %s\n\n", strings.ToUpper(deploymentMethod))
	}

	report += "The following gateway(s) are not connected and are using private IP (ESP protocol):\n"
	for _, issue := range gatewayIssues {
		report += fmt.Sprintf("  • %s\n", issue)
	}

	report += "\n"
	report += "Root Cause Analysis:\n"
	report += "  When NAT=no, Submariner uses the private IP address for IPSec tunnels, which means\n"
	report += "  IPSec traffic is encapsulated in ESP (IP protocol 50) instead of UDP port 4500.\n"
	report += "  If ESP protocol is not allowed in your firewall, the tunnel cannot be established.\n"
	report += "\n"
	report += "Recommended Actions:\n"
	report += "  1. Verify if ESP protocol (IP protocol 50) is blocked in your firewall by checking:\n"
	report += "     - Cloud provider security groups\n"
	report += "     - On-premise firewall rules\n"
	report += "     - iptables/nftables rules on the nodes\n"
	report += "\n"
	report += "  2. To confirm this is the issue, force UDP encapsulation on BOTH clusters:\n"
	report += "\n"
	report += "     IMPORTANT: This setting must be applied to ALL clusters in your ClusterSet!\n"
	report += "\n"

	// Provide deployment-specific instructions
	if deploymentMethod == "subctl" {
		report += "     Your deployment method: SUBCTL\n"
		report += "     \n"
		report += "     You can use the 'submariner_fix_esp_firewall' tool to automatically apply this fix,\n"
		report += "     or manually edit the Submariner CR on each cluster:\n"
		report += "     \n"
		report += "       kubectl patch submariner submariner -n submariner-operator --kubeconfig <cluster-kubeconfig> --type=merge -p '{\"spec\":{\"ceIPSecForceUDPEncaps\":true}}'\n"
		report += "     \n"
		report += "     Or manually edit:\n"
		report += "     \n"
		report += "       kubectl edit submariner submariner -n submariner-operator --kubeconfig <cluster-kubeconfig>\n"
		report += "     \n"
		report += "     Add the following to the spec section on EACH cluster:\n"
		report += "     \n"
		report += "       spec:\n"
		report += "         ceIPSecForceUDPEncaps: true\n"
		report += "     \n"
		report += "     After editing, restart the gateway pods on EACH cluster:\n"
		report += "     \n"
		report += "       kubectl delete pods -n submariner-operator -l app=submariner-gateway --kubeconfig <cluster-kubeconfig>\n"
		report += "\n"
	} else if deploymentMethod == "acm" {
		report += "     Your deployment method: RedHat ACM\n"
		report += "     \n"
		report += "     You can use the 'submariner_fix_esp_firewall' tool to automatically apply this fix,\n"
		report += "     or manually edit the SubmarinerConfig resource on EACH managed cluster:\n"
		report += "     \n"
		report += "       kubectl edit submarinerconfig <config-name> -n submariner-operator --kubeconfig <cluster-kubeconfig>\n"
		report += "     \n"
		report += "     Add the following to the spec section on EACH managed cluster:\n"
		report += "     \n"
		report += "       spec:\n"
		report += "         forceUDPEncaps: true\n"
		report += "     \n"
		report += "     After editing, restart the gateway pods on EACH managed cluster:\n"
		report += "     \n"
		report += "       kubectl delete pods -n submariner-operator -l app=submariner-gateway --kubeconfig <cluster-kubeconfig>\n"
		report += "\n"
	} else {
		report += "     Choose the appropriate method based on your deployment:\n"
		report += "     \n"
		report += "     Option A - If deployed using subctl:\n"
		report += "       kubectl patch submariner submariner -n submariner-operator --kubeconfig <cluster-kubeconfig> --type=merge -p '{\"spec\":{\"ceIPSecForceUDPEncaps\":true}}'\n"
		report += "       Then restart gateway pods: kubectl delete pods -n submariner-operator -l app=submariner-gateway --kubeconfig <cluster-kubeconfig>\n"
		report += "     \n"
		report += "     Option B - If deployed using RedHat ACM:\n"
		report += "       kubectl edit submarinerconfig <config-name> -n <namespace>\n"
		report += "       spec:\n"
		report += "         ceIPSecForceUDPEncaps: true\n"
		report += "\n"
	}

	report += "  3. If the tunnel comes up after setting ceIPSecForceUDPEncaps: true on both clusters:\n"
	report += "     - Keep this setting (recommended if you cannot allow ESP in firewall)\n"
	report += "     - Or allow ESP protocol (IP protocol 50) in your firewall and remove the setting\n"
	report += "\n"

	return report
}
