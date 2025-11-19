package submariner

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initSubmarinerFirewall() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "submariner_diagnose_firewall",
				Description: "Diagnose firewall configuration and inter-cluster tunnel connectivity between two Submariner clusters",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"from_kubeconfig": {
							Type:        "string",
							Description: "Path to the kubeconfig file for the source cluster (Required)",
						},
						"to_kubeconfig": {
							Type:        "string",
							Description: "Path to the kubeconfig file for the destination cluster (Optional, if not provided only checks source cluster)",
						},
						"from_context": {
							Type:        "string",
							Description: "Context name for the source cluster (Optional, uses current context from from_kubeconfig if not provided)",
						},
						"to_context": {
							Type:        "string",
							Description: "Context name for the destination cluster (Optional, uses current context from to_kubeconfig if not provided)",
						},
						"verbose": {
							Type:        "boolean",
							Description: "Include detailed output from subctl diagnose commands (Optional, default: false)",
							Default:     api.ToRawMessage(false),
						},
					},
					Required: []string{"from_kubeconfig"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Submariner: Diagnose Firewall Configuration",
					ReadOnlyHint:    ptr.To(false), // May create test pods for firewall validation
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: submarinerDiagnoseFirewall,
		},
	}
}

func submarinerDiagnoseFirewall(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse arguments
	fromKubeconfig, ok := params.GetArguments()["from_kubeconfig"].(string)
	if !ok || fromKubeconfig == "" {
		return api.NewToolCallResult("", fmt.Errorf("from_kubeconfig is required")), nil
	}

	toKubeconfig := ""
	if tk := params.GetArguments()["to_kubeconfig"]; tk != nil {
		toKubeconfig = tk.(string)
	}

	fromContext := ""
	if fc := params.GetArguments()["from_context"]; fc != nil {
		fromContext = fc.(string)
	}

	toContext := ""
	if tc := params.GetArguments()["to_context"]; tc != nil {
		toContext = tc.(string)
	}

	verbose := false
	if v := params.GetArguments()["verbose"]; v != nil {
		verbose = v.(bool)
	}

	// Build diagnostic report
	report := "=== Submariner Firewall Diagnostics ===\n"
	report += fmt.Sprintf("Source Cluster: %s\n", fromKubeconfig)
	if fromContext != "" {
		report += fmt.Sprintf("Source Context: %s\n", fromContext)
	}

	if toKubeconfig != "" {
		report += fmt.Sprintf("Destination Cluster: %s\n", toKubeconfig)
		if toContext != "" {
			report += fmt.Sprintf("Destination Context: %s\n", toContext)
		}
		report += "\n"

		// Run inter-cluster firewall diagnostics
		report += "=== Inter-Cluster Firewall Check ===\n"
		firewallOutput, err := runSubctlFirewallInterCluster(fromKubeconfig, toKubeconfig, fromContext, toContext, verbose)
		if err != nil {
			report += fmt.Sprintf("⚠ Firewall diagnostic completed with errors: %v\n", err)
			if firewallOutput != "" {
				report += "\nDiagnostic Output:\n" + firewallOutput + "\n"
			}
		} else {
			report += "✓ Firewall diagnostic completed successfully\n"
			report += "\nDiagnostic Output:\n" + firewallOutput + "\n"

			// Parse and summarize the output
			summary := parseSubctlOutput(firewallOutput)
			report += "\n=== Summary ===\n"
			if passed, ok := summary["checks_passed"].(int); ok && passed > 0 {
				report += fmt.Sprintf("✓ Checks passed: %d\n", passed)
			}
			if failed, ok := summary["checks_failed"].(int); ok && failed > 0 {
				report += fmt.Sprintf("✗ Checks failed: %d\n", failed)
			}
			if warnings, ok := summary["warnings"].(int); ok && warnings > 0 {
				report += fmt.Sprintf("⚠ Warnings: %d\n", warnings)
			}
		}
	} else {
		// Run local cluster diagnostics only
		report += "\n=== Single Cluster Diagnostics ===\n"
		report += "Running 'subctl diagnose all' on source cluster...\n\n"

		diagnoseOutput, err := runSubctlDiagnose(fromKubeconfig)
		if err != nil {
			report += fmt.Sprintf("⚠ Diagnostic completed with errors: %v\n", err)
			if diagnoseOutput != "" {
				report += "\nDiagnostic Output:\n" + diagnoseOutput + "\n"
			}
		} else {
			report += diagnoseOutput + "\n"

			// Parse and summarize the output
			summary := parseSubctlOutput(diagnoseOutput)
			report += "\n=== Summary ===\n"
			if passed, ok := summary["checks_passed"].(int); ok && passed > 0 {
				report += fmt.Sprintf("✓ Checks passed: %d\n", passed)
			}
			if failed, ok := summary["checks_failed"].(int); ok && failed > 0 {
				report += fmt.Sprintf("✗ Checks failed: %d\n", failed)
			}
			if warnings, ok := summary["warnings"].(int); ok && warnings > 0 {
				report += fmt.Sprintf("⚠ Warnings: %d\n", warnings)
			}
		}
	}

	return api.NewToolCallResult(report, nil), nil
}
