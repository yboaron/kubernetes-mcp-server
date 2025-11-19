package submariner

import (
	"strings"
	"testing"
)

func TestAnalyzeESPFirewallIssues_NoIssues(t *testing.T) {
	// Test case 1: All gateways connected with NAT=no (no issues)
	output := `
GATEWAY           CLUSTER    REMOTE IP    NAT   CABLE DRIVER   SUBNETS                        STATUS      RTT avg.
cluster2-worker   cluster2   172.18.0.6   no    libreswan      100.67.0.0/16, 10.131.0.0/16   connected   423.415µs
cluster3-worker   cluster3   172.18.0.8   no    libreswan      100.68.0.0/16, 10.132.0.0/16   connected   523.415µs
`

	result := analyzeESPFirewallIssues(output, "unknown")
	if !strings.Contains(result, "✓ No ESP firewall issues detected") {
		t.Errorf("Expected no issues, but got: %s", result)
	}
}

func TestAnalyzeESPFirewallIssues_WithESPIssue_Subctl(t *testing.T) {
	// Test case 2a: Gateway not connected with NAT=no (ESP issue) - subctl deployment
	output := `
GATEWAY           CLUSTER    REMOTE IP    NAT   CABLE DRIVER   SUBNETS                        STATUS          RTT avg.
cluster2-worker   cluster2   172.18.0.6   no    libreswan      100.67.0.0/16, 10.131.0.0/16   connecting      -
cluster3-worker   cluster3   172.18.0.8   yes   libreswan      100.68.0.0/16, 10.132.0.0/16   connected       523.415µs
`

	result := analyzeESPFirewallIssues(output, "subctl")
	if !strings.Contains(result, "⚠ POTENTIAL ESP FIREWALL ISSUE DETECTED") {
		t.Errorf("Expected ESP firewall issue detection, but got: %s", result)
	}
	if !strings.Contains(result, "cluster2-worker") {
		t.Errorf("Expected cluster2-worker in the report, but got: %s", result)
	}
	if !strings.Contains(result, "forceUDPEncaps: true") {
		t.Errorf("Expected forceUDPEncaps recommendation, but got: %s", result)
	}
	if !strings.Contains(result, "Your deployment method: SUBCTL") {
		t.Errorf("Expected SUBCTL deployment method, but got: %s", result)
	}
	if !strings.Contains(result, "submariner_fix_esp_firewall") {
		t.Errorf("Expected reference to fix tool, but got: %s", result)
	}
	if !strings.Contains(result, "kubectl delete pods -n submariner-operator -l app=submariner-gateway") {
		t.Errorf("Expected gateway pod restart command, but got: %s", result)
	}
}

func TestAnalyzeESPFirewallIssues_WithESPIssue_ACM(t *testing.T) {
	// Test case 2b: Gateway not connected with NAT=no (ESP issue) - ACM deployment
	output := `
GATEWAY           CLUSTER    REMOTE IP    NAT   CABLE DRIVER   SUBNETS                        STATUS          RTT avg.
cluster2-worker   cluster2   172.18.0.6   no    libreswan      100.67.0.0/16, 10.131.0.0/16   connecting      -
`

	result := analyzeESPFirewallIssues(output, "acm")
	if !strings.Contains(result, "⚠ POTENTIAL ESP FIREWALL ISSUE DETECTED") {
		t.Errorf("Expected ESP firewall issue detection, but got: %s", result)
	}
	if !strings.Contains(result, "Your deployment method: RedHat ACM") {
		t.Errorf("Expected ACM deployment method, but got: %s", result)
	}
	if !strings.Contains(result, "submarinerconfig") {
		t.Errorf("Expected SubmarinerConfig reference, but got: %s", result)
	}
}

func TestAnalyzeESPFirewallIssues_MixedNATStatus(t *testing.T) {
	// Test case 3: Mixed NAT status, only NAT=no and not connected should trigger issue
	output := `
GATEWAY           CLUSTER    REMOTE IP    NAT   CABLE DRIVER   SUBNETS                        STATUS          RTT avg.
cluster2-worker   cluster2   172.18.0.6   no    libreswan      100.67.0.0/16, 10.131.0.0/16   error           -
cluster3-worker   cluster3   172.18.0.8   yes   libreswan      100.68.0.0/16, 10.132.0.0/16   connecting      -
cluster4-worker   cluster4   172.18.0.9   no    libreswan      100.69.0.0/16, 10.133.0.0/16   connected       623.415µs
`

	result := analyzeESPFirewallIssues(output, "unknown")
	if !strings.Contains(result, "⚠ POTENTIAL ESP FIREWALL ISSUE DETECTED") {
		t.Errorf("Expected ESP firewall issue detection, but got: %s", result)
	}
	if !strings.Contains(result, "cluster2-worker") {
		t.Errorf("Expected cluster2-worker in the report, but got: %s", result)
	}
	// cluster3-worker should NOT be in the report (NAT=yes)
	if strings.Contains(result, "cluster3-worker") {
		t.Errorf("cluster3-worker should not be in the report (NAT=yes), but got: %s", result)
	}
	// cluster4-worker should NOT be in the report (connected)
	if strings.Contains(result, "cluster4-worker") {
		t.Errorf("cluster4-worker should not be in the report (connected), but got: %s", result)
	}
}

func TestAnalyzeESPFirewallIssues_EmptyOutput(t *testing.T) {
	// Test case 4: Empty output
	result := analyzeESPFirewallIssues("", "unknown")
	if result != "" {
		t.Errorf("Expected empty result for empty input, but got: %s", result)
	}
}
