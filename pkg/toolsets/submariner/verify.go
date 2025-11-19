package submariner

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

const (
	defaultSmallPacketSize = 200
)

func initSubmarinerVerify() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "submariner_verify_datapath",
				Description: "Verify inter-cluster datapath connectivity between two Submariner clusters with automatic MTU and source IP validation troubleshooting",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"from_kubeconfig": {
							Type:        "string",
							Description: "Path to the kubeconfig file for the source cluster (Required)",
						},
						"to_kubeconfig": {
							Type:        "string",
							Description: "Path to the kubeconfig file for the destination cluster (Required)",
						},
						"from_context": {
							Type:        "string",
							Description: "Context name for the source cluster (Optional, uses current context from from_kubeconfig if not provided)",
						},
						"to_context": {
							Type:        "string",
							Description: "Context name for the destination cluster (Optional, uses current context from to_kubeconfig if not provided)",
						},
						"auto_troubleshoot": {
							Type:        "boolean",
							Description: "Automatically run MTU and source IP validation tests if connectivity fails (Optional, default: true)",
							Default:     api.ToRawMessage(true),
						},
						"verbose": {
							Type:        "boolean",
							Description: "Include detailed output from subctl verify commands (Optional, default: false)",
							Default:     api.ToRawMessage(false),
						},
					},
					Required: []string{"from_kubeconfig", "to_kubeconfig"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Submariner: Verify Inter-Cluster Datapath",
					ReadOnlyHint:    ptr.To(false), // Creates test pods
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: submarinerVerifyDatapath,
		},
	}
}

func submarinerVerifyDatapath(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse arguments
	fromKubeconfig, ok := params.GetArguments()["from_kubeconfig"].(string)
	if !ok || fromKubeconfig == "" {
		return api.NewToolCallResult("", fmt.Errorf("from_kubeconfig is required")), nil
	}

	toKubeconfig, ok := params.GetArguments()["to_kubeconfig"].(string)
	if !ok || toKubeconfig == "" {
		return api.NewToolCallResult("", fmt.Errorf("to_kubeconfig is required")), nil
	}

	fromContext := ""
	if fc := params.GetArguments()["from_context"]; fc != nil {
		fromContext = fc.(string)
	}

	toContext := ""
	if tc := params.GetArguments()["to_context"]; tc != nil {
		toContext = tc.(string)
	}

	autoTroubleshoot := true
	if at := params.GetArguments()["auto_troubleshoot"]; at != nil {
		autoTroubleshoot = at.(bool)
	}

	verbose := false
	if v := params.GetArguments()["verbose"]; v != nil {
		verbose = v.(bool)
	}

	// Build verification report
	report := "=== Submariner Inter-Cluster Datapath Verification ===\n"
	report += fmt.Sprintf("From: %s\n", fromKubeconfig)
	report += fmt.Sprintf("To: %s\n", toKubeconfig)
	if fromContext != "" {
		report += fmt.Sprintf("From Context: %s\n", fromContext)
	}
	if toContext != "" {
		report += fmt.Sprintf("To Context: %s\n", toContext)
	}
	report += "\n"

	// 1. Run basic connectivity test
	report += "=== Running Basic Connectivity Test ===\n"
	connectivityResult, err := runSubctlVerifyConnectivity(fromKubeconfig, toKubeconfig, fromContext, toContext, 0, false, verbose)
	if err != nil {
		report += fmt.Sprintf("⚠ Error running connectivity test: %v\n\n", err)
		return api.NewToolCallResult(report, nil), nil
	}

	report += connectivityResult.Summary + "\n"
	if verbose && connectivityResult.FullOutput != "" {
		report += "\nDetailed Output:\n" + connectivityResult.FullOutput + "\n"
	}

	// 2. If tests failed and auto-troubleshooting is enabled, run diagnostics
	if !connectivityResult.AllPassed && autoTroubleshoot {
		report += "\n=== Automated Troubleshooting ===\n"
		report += "Some connectivity tests failed. Running diagnostic tests...\n\n"

		// Test with small packet size to detect MTU issues
		report += "--- Testing with Small Packet Size (MTU Diagnostic) ---\n"
		mtuResult, err := runSubctlVerifyConnectivity(fromKubeconfig, toKubeconfig, fromContext, toContext, defaultSmallPacketSize, false, verbose)
		if err != nil {
			report += fmt.Sprintf("⚠ Error running MTU diagnostic: %v\n\n", err)
		} else {
			report += mtuResult.Summary + "\n"
			if verbose && mtuResult.FullOutput != "" {
				report += "\nDetailed Output:\n" + mtuResult.FullOutput + "\n"
			}

			if mtuResult.AllPassed && !connectivityResult.AllPassed {
				report += "\n🔍 DIAGNOSIS: MTU Issue Detected\n"
				report += "Tests pass with small packets (200 bytes) but fail with default packet size (~3000 bytes).\n"
				report += "This indicates an MTU (Maximum Transmission Unit) problem in the network path.\n"
				report += "\nRoot Cause:\n"
				report += "  Submariner adds IPSec encapsulation overhead (~60-80 bytes) which reduces the\n"
				report += "  effective MTU. When large packets (~3000 bytes used in e2e tests) exceed the\n"
				report += "  path MTU, they are fragmented or dropped.\n"
				report += "\nRecommended Workaround - TCP MSS Clamping:\n"
				report += "  Apply TCP MSS clamping by annotating the gateway node(s). This limits the maximum\n"
				report += "  segment size for TCP connections, preventing packet fragmentation.\n"
				report += "\n"
				report += "  1. Annotate the gateway node with TCP MSS clamping value (e.g., 1200):\n"
				report += "     kubectl annotate node <gateway-node-name> submariner.io/tcp-clamp-mss=1200\n"
				report += "     \n"
				report += "     or for OpenShift:\n"
				report += "     oc annotate node <gateway-node-name> submariner.io/tcp-clamp-mss=1200\n"
				report += "\n"
				report += "  2. Restart all RouteAgent pods to apply the configuration:\n"
				report += "     kubectl delete pod -n submariner-operator -l app=submariner-routeagent\n"
				report += "     \n"
				report += "     or for OpenShift:\n"
				report += "     oc delete pod -n submariner-operator -l app=submariner-routeagent\n"
				report += "\n"
				report += "  3. Repeat for all gateway nodes in the cluster\n\n"
			}
		}

		// Test with source IP validation disabled to detect source IP issues
		report += "--- Testing with Source IP Validation Disabled ---\n"
		srcIPResult, err := runSubctlVerifyConnectivity(fromKubeconfig, toKubeconfig, fromContext, toContext, 0, true, verbose)
		if err != nil {
			report += fmt.Sprintf("⚠ Error running source IP diagnostic: %v\n\n", err)
		} else {
			report += srcIPResult.Summary + "\n"
			if verbose && srcIPResult.FullOutput != "" {
				report += "\nDetailed Output:\n" + srcIPResult.FullOutput + "\n"
			}

			if srcIPResult.AllPassed && !connectivityResult.AllPassed {
				report += "\n🔍 DIAGNOSIS: Source IP Validation Issue Detected\n"
				report += "Tests pass with source IP validation disabled but fail with validation enabled.\n"
				report += "This is a known issue with Submariner and OVN-Kubernetes CNI.\n"
				report += "\nRoot Cause:\n"
				report += "Submariner has a bug with OVNK CNI where source IP addresses are not properly\n"
				report += "preserved during cross-cluster communication, causing validation failures.\n"
				report += "\nRecommended Actions:\n"
				report += "1. Check if you're using OVN-Kubernetes as your CNI\n"
				report += "2. Verify Submariner version and check for updates/patches\n"
				report += "3. Review Submariner issue tracker for OVNK-specific fixes\n"
				report += "4. Consider using --skip-src-ip-check as a workaround if acceptable\n"
				report += "5. Test with alternative CNI plugins if possible\n\n"
			}
		}

		// Provide general troubleshooting summary
		report += "=== Troubleshooting Summary ===\n"
		if connectivityResult.AllPassed {
			report += "✓ All connectivity tests passed - no issues detected\n"
		} else if mtuResult.AllPassed {
			report += "⚠ MTU issue detected - review MTU configuration\n"
		} else if srcIPResult.AllPassed {
			report += "⚠ Source IP validation issue detected - known OVNK CNI bug\n"
		} else {
			report += "✗ Connectivity issues persist across all tests\n"
			report += "Additional troubleshooting steps:\n"
			report += "1. Verify Submariner gateway connectivity (check gateway pod logs)\n"
			report += "2. Check firewall rules and network policies\n"
			report += "3. Verify cable driver (libreswan/wireguard) configuration\n"
			report += "4. Run 'subctl diagnose all' on both clusters\n"
			report += "5. Check for network segmentation or routing issues\n"
		}
	} else if connectivityResult.AllPassed {
		report += "\n✓ All connectivity tests passed successfully!\n"
	}

	return api.NewToolCallResult(report, nil), nil
}

type connectivityTestResult struct {
	AllPassed  bool
	Summary    string
	FullOutput string
}

// runSubctlVerifyConnectivity executes subctl verify with connectivity tests
func runSubctlVerifyConnectivity(fromKubeconfig, toKubeconfig, fromContext, toContext string, packetSize int, skipSrcIPCheck bool, verbose bool) (*connectivityTestResult, error) {
	subctlPath, err := findSubctl()
	if err != nil {
		return nil, err
	}

	args := []string{"verify"}

	// Add kubeconfig for source cluster
	if fromContext != "" {
		args = append(args, "--context", fromContext)
	}
	args = append(args, "--kubeconfig", fromKubeconfig)

	// Add kubeconfig for destination cluster
	if toContext != "" {
		args = append(args, "--tocontext", toContext)
	}
	args = append(args, "--toconfig", toKubeconfig)

	// Only run connectivity tests
	args = append(args, "--only", "connectivity")

	// Add optional flags
	if packetSize > 0 {
		args = append(args, "--packet-size", fmt.Sprintf("%d", packetSize))
	}
	if skipSrcIPCheck {
		args = append(args, "--skip-src-ip-check")
	}

	// Add verbose flag for detailed output
	if verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.Command(subctlPath, args...)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Parse the output to determine if all tests passed
	allPassed := err == nil && !strings.Contains(outputStr, "failed") && !strings.Contains(outputStr, "error")

	// Build summary
	summary := ""
	if packetSize > 0 {
		summary += fmt.Sprintf("Test with packet size %d bytes: ", packetSize)
	} else if skipSrcIPCheck {
		summary += "Test with source IP validation disabled: "
	} else {
		summary += "Standard connectivity test: "
	}

	if allPassed {
		summary += "✓ All tests passed"
	} else {
		summary += "✗ Some tests failed"
		if err != nil {
			summary += fmt.Sprintf(" (exit code: non-zero)")
		}
	}

	// Count passed/failed tests if possible
	passedCount := strings.Count(outputStr, "✓")
	failedCount := strings.Count(outputStr, "✗")
	if passedCount > 0 || failedCount > 0 {
		summary += fmt.Sprintf(" (%d passed, %d failed)", passedCount, failedCount)
	}

	result := &connectivityTestResult{
		AllPassed:  allPassed,
		Summary:    summary,
		FullOutput: outputStr,
	}

	return result, nil
}
