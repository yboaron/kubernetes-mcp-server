# Submariner Toolset for Kubernetes MCP Server

This toolset adds Submariner multi-cluster connectivity monitoring capabilities to the Kubernetes MCP Server.

## Features

The Submariner toolset provides comprehensive health checking for Submariner deployments:

1. **Pod Status Verification** - Checks that all Submariner pods are running
2. **Log Analysis** - Analyzes pod logs for errors and warnings
3. **Subctl Integration** - Executes `subctl show all` and `subctl diagnose all` commands
4. **Comprehensive Reporting** - Provides detailed health reports

## Prerequisites

1. **Submariner** installed in your Kubernetes cluster
2. **subctl** binary - **automatically installed** if not found in your system
   - The toolset will attempt to install subctl automatically using the official installation script
   - Manual installation: https://submariner.io/operations/deployment/subctl/
   - Installation location: `~/.local/bin/subctl`

## Installation

The Submariner toolset is already included in the build. To enable it:

### Option 1: Enable via Command Line

```bash
./kubernetes-mcp-server --toolsets=core,submariner --kubeconfig=/path/to/kubeconfig
```

### Option 2: Enable in Claude Code Config

Update your `~/.claude/config.json`:

```json
{
  "mcpServers": {
    "kubernetes": {
      "command": "/home/yboaron/prj/kubernetes-mcp-server/kubernetes-mcp-server",
      "args": [
        "--toolsets=core,submariner",
        "--kubeconfig=/home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster1"
      ]
    }
  }
}
```

## Available Tools

### 1. submariner_health_check

Comprehensive health check for Submariner deployment.

**Parameters:**

- `namespace` (string, optional): Namespace where Submariner is installed (default: "submariner-operator")
- `kubeconfig_path` (string, optional): Path to the kubeconfig file
- `include_subctl_show` (boolean, optional): Include `subctl show all` output (default: true)
- `include_subctl_diagnose` (boolean, optional): Include `subctl diagnose all` output (default: true)
- `check_logs` (boolean, optional): Analyze pod logs for errors (default: true)

**Example Usage via Claude:**

```
Check the health of Submariner in my cluster
```

or more specifically:

```
Run a Submariner health check with namespace submariner-operator and kubeconfig path /home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster1
```

**Example Tool Call (Direct MCP):**

```json
{
  "name": "submariner_health_check",
  "arguments": {
    "namespace": "submariner-operator",
    "kubeconfig_path": "/home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster1",
    "include_subctl_show": true,
    "include_subctl_diagnose": true,
    "check_logs": true
  }
}
```

## Health Check Report Format

The tool provides a comprehensive report with the following sections:

### 1. Pod Status
```
=== Pod Status ===
✓ submariner-gateway-78v7z: Running (1/1 ready)
✓ submariner-metrics-proxy-58vcq: Running (1/1 ready)
✓ submariner-operator-7778ddf55c-cq8lj: Running (1/1 ready)
✓ submariner-routeagent-4q8x2: Running (1/1 ready)
✓ submariner-routeagent-z77fk: Running (1/1 ready)

✓ All 5 Submariner pods are healthy
```

### 2. Log Analysis
```
=== Log Analysis ===
✓ No errors found in pod logs (last 100 lines checked per pod)
```

or if errors are found:

```
=== Log Analysis ===
⚠ submariner-gateway-78v7z: Found 3 error(s) in logs (last 100 lines)
  Sample errors:
    - Error: failed to connect to remote endpoint
    - Error: timeout waiting for response
```

### 3. Subctl Show All
```
=== Subctl Show All ===
Cluster "cluster1"
 ✓ Detecting broker(s)
NAMESPACE               NAME                COMPONENTS     GLOBALNET   GLOBALNET CIDR
submariner-k8s-broker   submariner-broker   connectivity   no          242.0.0.0/8

 ✓ Showing Connections
GATEWAY           CLUSTER    REMOTE IP    NAT   CABLE DRIVER   SUBNETS                        STATUS      RTT avg.
cluster2-worker   cluster2   172.18.0.6   no    libreswan      100.67.0.0/16, 10.131.0.0/16   connected   141.395µs
```

### 4. Subctl Diagnose All
```
=== Subctl Diagnose All ===
Cluster "cluster1"
 ✓ Checking Submariner support for the Kubernetes version
 ✓ Kubernetes version "v1.34.0" is supported
 ✓ Non-Globalnet deployment detected - checking that cluster CIDRs do not overlap
 ✓ Checking DaemonSet "submariner-gateway"
 ✓ Checking DaemonSet "submariner-routeagent"
 ✓ Checking the status of all Submariner pods
 ✓ Checking Submariner support for the CNI network plugin
 ✓ The detected CNI network plugin ("kindnet") is supported
 ✓ Checking gateway connections
```

## Troubleshooting

### "subctl binary not found"

If you see this error, install subctl:

```bash
# Download and install subctl
curl -Ls https://get.submariner.io | bash
export PATH=$PATH:~/.local/bin
```

### "failed to list pods in namespace submariner-operator"

This usually means Submariner is not installed or installed in a different namespace. Check:

```bash
kubectl get namespaces | grep submariner
```

### 2. submariner_verify_datapath

Verify inter-cluster datapath connectivity with automatic troubleshooting for MTU and source IP validation issues.

**Parameters:**

- `from_kubeconfig` (string, required): Path to the kubeconfig file for the source cluster
- `to_kubeconfig` (string, required): Path to the kubeconfig file for the destination cluster
- `from_context` (string, optional): Context name for the source cluster
- `to_context` (string, optional): Context name for the destination cluster
- `auto_troubleshoot` (boolean, optional): Automatically run MTU and source IP validation tests if connectivity fails (default: true)
- `verbose` (boolean, optional): Include detailed output from subctl verify commands (default: false)

**Example Usage via Claude:**

```
Verify inter-cluster datapath between cluster1 and cluster2
```

or more specifically:

```
Verify Submariner datapath from /home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster1 to /home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster2
```

**What it does:**

1. **Basic Connectivity Test**: Runs `subctl verify --only connectivity` to test datapath between clusters
2. **MTU Issue Detection**: If tests fail, automatically runs tests with small packet size (200 bytes) to detect MTU problems
3. **Source IP Validation**: Tests with `--skip-src-ip-check` to identify OVNK CNI source IP validation bugs
4. **Automatic Diagnosis**: Provides clear diagnosis and recommended actions based on test results

**Diagnostic Output:**

The tool provides intelligent troubleshooting:

- **MTU Issues**: If tests pass with small packets but fail with default size, identifies MTU problems and provides configuration guidance
- **Source IP Issues**: If tests pass with source IP validation disabled, identifies the known OVNK CNI bug and provides workarounds
- **General Failures**: If all tests fail, provides additional troubleshooting steps

**Example Tool Call (Direct MCP):**

```json
{
  "name": "submariner_verify_datapath",
  "arguments": {
    "from_kubeconfig": "/home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster1",
    "to_kubeconfig": "/home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster2",
    "auto_troubleshoot": true,
    "verbose": false
  }
}
```

### 3. submariner_diagnose_firewall

Diagnose firewall configuration and inter-cluster tunnel connectivity between Submariner clusters.

**Parameters:**

- `from_kubeconfig` (string, required): Path to the kubeconfig file for the source cluster
- `to_kubeconfig` (string, optional): Path to the kubeconfig file for the destination cluster (if not provided, runs diagnostics on source cluster only)
- `from_context` (string, optional): Context name for the source cluster
- `to_context` (string, optional): Context name for the destination cluster
- `verbose` (boolean, optional): Include detailed output (default: false)

**Example Usage via Claude:**

```
Diagnose firewall between cluster1 and cluster2
```

or more specifically:

```
Check Submariner firewall configuration from /home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster1 to /home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster2
```

**What it does:**

1. **Inter-Cluster Firewall Tests**: When both kubeconfigs are provided, runs `subctl diagnose firewall inter-cluster` to validate tunnels can be established
2. **Single Cluster Diagnostics**: When only source kubeconfig is provided, runs `subctl diagnose all` on the source cluster
3. **Tunnel Validation**: Checks if IPsec/WireGuard tunnels can be set up on gateway nodes
4. **Summary Report**: Provides counts of passed/failed checks and warnings

**Example Tool Call (Direct MCP):**

```json
{
  "name": "submariner_diagnose_firewall",
  "arguments": {
    "from_kubeconfig": "/home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster1",
    "to_kubeconfig": "/home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster2",
    "verbose": true
  }
}
```

## Command Reference

All `subctl` commands executed by the toolset:

### Health Check Commands
- `subctl show all --kubeconfig <path>` - Shows Submariner deployment status, gateways, endpoints, and connections
- `subctl diagnose all --kubeconfig <path>` - Runs comprehensive diagnostics on Submariner installation

### Datapath Verification Commands
- `subctl verify --kubeconfig <from> --toconfig <to> --only connectivity` - Tests basic inter-cluster connectivity
- `subctl verify --kubeconfig <from> --toconfig <to> --only connectivity --packet-size 200` - Tests with small packets (MTU diagnostic)
- `subctl verify --kubeconfig <from> --toconfig <to> --only connectivity --skip-src-ip-check` - Tests without source IP validation

### Firewall Diagnostic Commands
- `subctl diagnose firewall inter-cluster --kubeconfig <from> --remoteconfig <to>` - Validates firewall configuration and tunnel establishment between clusters

## Automatic Subctl Installation

The toolset automatically installs `subctl` if it's not found in your system:

1. **Installation Method**: Uses the official Submariner installation script from `https://get.submariner.io`
2. **Installation Location**: `~/.local/bin/subctl` (automatically added to PATH)
3. **Version**: Installs the latest stable release by default
4. **Supported Platforms**: Linux and macOS
5. **Manual Installation**: If automatic installation fails, install manually:
   ```bash
   curl -Ls https://get.submariner.io | bash
   export PATH=$PATH:~/.local/bin
   echo export PATH=\$PATH:~/.local/bin >> ~/.profile
   ```

### 4. submariner_fix_esp_firewall

**NEW**: Automatically detect and fix ESP firewall issues by applying ceIPSecForceUDPEncaps to Submariner deployments on both clusters (subctl deployments only).

**Parameters:**

- `cluster1_kubeconfig` (string, required): Path to the kubeconfig file for the first cluster
- `cluster2_kubeconfig` (string, required): Path to the kubeconfig file for the second cluster
- `namespace` (string, optional): Namespace where Submariner is installed (default: "submariner-operator")
- `verify_timeout_seconds` (number, optional): Timeout in seconds to wait for tunnel to come up after fix (default: 60)

**Example Usage via Claude:**

```
Fix ESP firewall issues between cluster1 and cluster2
```

or more specifically:

```
Apply ESP firewall fix to Submariner using /path/to/cluster1/kubeconfig and /path/to/cluster2/kubeconfig
```

**What it does:**

1. **Deployment Detection**: Verifies both clusters are subctl deployments (not ACM)
2. **CR Patching**: Patches Submariner CR on both clusters to add `ceIPSecForceUDPEncaps: true`
3. **Pod Restart**: Restarts gateway pods on both clusters
4. **Verification**: Waits up to 60 seconds and verifies tunnel comes up as "connected"

**Example Tool Call (Direct MCP):**

```json
{
  "name": "submariner_fix_esp_firewall",
  "arguments": {
    "cluster1_kubeconfig": "/home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster1",
    "cluster2_kubeconfig": "/home/yboaron/prj/submariner/output/kubeconfigs/kind-config-cluster2",
    "verify_timeout_seconds": 60
  }
}
```

## ESP Firewall Issue Detection and Remediation

### Problem Statement

When Submariner establishes IPSec tunnels between clusters, it uses NAT-Discovery (NAT-D) to determine which IP address to use:

- **NAT=no (Private IP)**: IPSec traffic is encapsulated in **ESP (IP protocol 50)**
- **NAT=yes (Public IP)**: IPSec traffic is encapsulated in **UDP port 4500**

If ESP protocol is blocked by firewall but NAT-D selects private IP, the tunnel cannot be established.

### Automatic Detection

The `submariner_health_check` tool now automatically:
- Detects deployment method (subctl vs ACM) by checking for `submariner-addon` pod
- Analyzes the active Gateway CR's `status.connections[]` to determine if private IP is being used
- Checks if `usingIP` matches `endpoint.private_ip` (instead of `endpoint.public_ip`)
- Identifies connections that are using private IP and not in "connected" status
- Provides deployment-specific remediation instructions

**Detection Method:**

The tool examines the Gateway custom resource (CR) to detect ESP firewall issues:

1. Gets the active Gateway resource (where `status.haStatus: active`)
2. For each connection in `status.connections[]`:
   - Extracts `usingIP`, `endpoint.private_ip`, and `endpoint.public_ip`
   - Checks if `usingIP == endpoint.private_ip` (indicating ESP protocol)
   - Checks if `status != "connected"`
3. If both conditions are met, identifies it as a potential ESP firewall issue

This method is more reliable than parsing `subctl show` output or analyzing logs, as the Gateway CR is the authoritative source of truth for NAT-Discovery decisions.

**Example Detection Output:**

```
=== ESP/Firewall Analysis ===
⚠ POTENTIAL ESP FIREWALL ISSUE DETECTED

Deployment Method Detected: SUBCTL

Active Gateway: cluster2-worker

The following connection(s) are not connected and using private IP (ESP protocol):
  • Cluster 'cluster1': Status=error, Using IP=172.18.0.4 (private), Public IP=80.230.113.10

Root Cause Analysis:
  When the gateway selects private IP (usingIP = private_ip), IPSec traffic is
  encapsulated in ESP (IP protocol 50) instead of UDP port 4500.
  If ESP protocol is not allowed in your firewall, the tunnel cannot be established.

Recommended Actions:
  1. Verify if ESP protocol (IP protocol 50) is blocked in your firewall
  2. You can use the 'submariner_fix_esp_firewall' tool to automatically apply this fix
```

### Automatic Remediation

The `submariner_fix_esp_firewall` tool automatically applies the fix:

**Example Remediation Output:**

```
=== Submariner ESP Firewall Remediation ===

Step 1: Verifying deployment method...
  ✓ Cluster1: subctl deployment detected
  ✓ Cluster2: subctl deployment detected

Step 2: Checking current tunnel status...
  Current status captured

Step 3: Applying ceIPSecForceUDPEncaps to Cluster1...
  ✓ ceIPSecForceUDPEncaps: true applied to cluster1

Step 4: Applying ceIPSecForceUDPEncaps to Cluster2...
  ✓ ceIPSecForceUDPEncaps: true applied to cluster2

Step 5: Restarting gateway pods on Cluster1...
  ✓ Gateway pods restarted on cluster1

Step 6: Restarting gateway pods on Cluster2...
  ✓ Gateway pods restarted on cluster2

Step 7: Waiting up to 60 seconds for tunnel to come up...
  ✓ SUCCESS: Tunnel is now connected!

The ESP firewall issue has been resolved. The IPSec tunnel is now using
UDP encapsulation (port 4500) instead of ESP protocol.
```

### Manual Remediation

#### For subctl Deployments

1. Edit the Submariner CR on **EACH** cluster:
```bash
kubectl edit submariner submariner -n submariner-operator --kubeconfig <cluster-kubeconfig>
```

2. Add to spec section:
```yaml
spec:
  ceIPSecForceUDPEncaps: true
```

3. Restart gateway pods on **EACH** cluster:
```bash
kubectl delete pods -n submariner-operator -l app=submariner-gateway --kubeconfig <cluster-kubeconfig>
```

#### For ACM Deployments

1. Edit SubmarinerConfig on ACM HUB cluster:
```bash
kubectl edit submarinerconfig <config-name> -n <namespace>
```

2. Add to spec section (applies to all managed clusters):
```yaml
spec:
  ceIPSecForceUDPEncaps: true
```

### Technical Details

**Deployment Method Detection:**
- **Present** `submariner-addon` pod: ACM deployment
- **Missing** `submariner-addon` pod: subctl deployment

**Submariner CR Patching:**
```bash
kubectl patch submariner submariner -n submariner-operator \
  --type=merge \
  -p '{"spec":{"ceIPSecForceUDPEncaps":true}}'
```

**Gateway Pod Restart:**
```bash
kubectl delete pods -n submariner-operator -l app=submariner-gateway
```

**Tunnel Status Verification:**
- Polls `subctl show all` every 5 seconds
- Checks if tunnel status becomes "connected"
- Default timeout: 60 seconds

### **UPDATE**: ACM Support Added

The tool now fully supports **both subctl and ACM deployments**:

- **For subctl**: Sets `ceIPSecForceUDPEncaps: true` in Submariner CR on each cluster
- **For ACM**: Sets `forceUDPEncaps: true` in SubmarinerConfig on each managed cluster

### Important Notes

- The fix is **idempotent** - safe to run multiple times
- Both clusters must be accessible with their respective kubeconfigs
- The setting must be applied on **ALL clusters** in the ClusterSet
- For ACM: Uses `forceUDPEncaps` in SubmarinerConfig (not `ceIPSecForceUDPEncaps`)
- For subctl: Uses `ceIPSecForceUDPEncaps` in Submariner CR
- If tunnel doesn't come up after fix, check that UDP port 4500 is allowed

## Future Enhancements

Planned features:

1. **Gateway status monitoring** - Monitor active/passive gateway status changes over time
2. **Service discovery checks** - Verify Lighthouse service discovery functionality
3. **Network policy validation** - Check network policies for Submariner traffic
4. **Historical connectivity tracking** - Track connectivity status over time
5. **ACM deployment auto-remediation** - Extend ESP fix to support ACM deployments

## Contributing

To add more Submariner tools, create new functions in `pkg/toolsets/submariner/` following the existing pattern.
