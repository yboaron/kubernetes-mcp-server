# Submariner ESP Firewall Check and Fix

You are testing and fixing ESP protocol firewall blocking by applying UDP encapsulation.

## Your Role

The `/submariner-tunnel-troubleshoot` command identified that ESP protocol (IP 50) blocking is the likely root cause. Your job is to:
1. Apply the UDP encapsulation fix on both clusters
2. Restart gateway pods
3. Verify if the tunnel comes up
4. Report success or recommend next steps

## Background

Submariner runs **NAT discovery** to test connectivity to both private and public IPs.
If the private IP responds, Submariner will use it for the tunnel, even if NAT is configured.

When Submariner uses private IPs, IPsec traffic is encapsulated in **ESP (IP protocol 50)**.
If ESP is blocked in the firewall, tunnels cannot establish.

The fix: Force Submariner to use **UDP encapsulation (port 4500)** instead of ESP.
This forces all IPSec traffic to use UDP, regardless of which IP (private or public) was selected.

## Your Task

### Phase 1: Get Cluster Information and Detect Deployment Type

**This command fixes ESP blocking for the tunnel between TWO clusters.**

**Check if kubeconfigs were provided as parameters:**
- This command can be invoked as `/submariner-tunnel-esp-check <kubeconfig1> <kubeconfig2>`
- If both kubeconfig paths are provided as parameters, use them directly
- If not provided, ask the user for them

**Important for multi-cluster mesh scenarios:**
- Submariner creates full-mesh tunnels (e.g., 3 clusters = 3 tunnels: A↔B, A↔C, B↔C)
- If ESP is the issue, it typically affects **ALL tunnels using private IPs**
- This command applies the fix for **ONE tunnel at a time** (between two clusters)
- If you have 3 clusters and all tunnels are failing due to ESP:
  - Run this command for the first pair: `/submariner-tunnel-esp-check <clusterA-config> <clusterB-config>`
  - Then run it again for the next pair: `/submariner-tunnel-esp-check <clusterA-config> <clusterC-config>`
  - Then run it for the final pair: `/submariner-tunnel-esp-check <clusterB-config> <clusterC-config>`
  - Alternatively, after fixing one tunnel, you can manually apply the fix to remaining clusters

Ask the user for (if not provided as parameters):
- Kubeconfig path for **cluster 1**
- Kubeconfig path for **cluster 2**
- Custom namespace (optional, default: submariner-operator)

**Detect Deployment Type:**

Check if this is an ACM (Advanced Cluster Management) deployment:
```bash
kubectl get pods -n submariner-operator -l app=submariner-addon --kubeconfig <cluster-kubeconfig>
```

- **If submariner-addon pod exists**: This is an ACM deployment → Go to Phase 1-ACM
- **If submariner-addon pod does NOT exist**: This is a subctl deployment → Go to Phase 1-Subctl

### Phase 1-ACM: For ACM Deployments

**Step 1: Get ACM Hub Kubeconfig**

Ask the user for the ACM hub cluster kubeconfig:
```
Your clusters are managed by ACM (Advanced Cluster Management).
To apply the fix, I need the kubeconfig for the ACM hub cluster.

Please provide the path to the ACM hub kubeconfig:
```

**Step 2: List SubmarinerConfig Resources**

List all SubmarinerConfig resources across all namespaces on the ACM hub:
```bash
kubectl get submarinerconfig --all-namespaces --kubeconfig <hub-kubeconfig>
```

Expected output format:
```
NAMESPACE                NAME                AGE
cluster1-namespace       submariner-config   5d
cluster2-namespace       submariner-config   5d
cluster3-namespace       submariner-config   5d
```

**Step 3: Ask User to Select SubmarinerConfig CRs**

Present the list to the user and ask them to select the TWO SubmarinerConfig CRs corresponding to the two managed clusters in the failing tunnel:

```
Found the following SubmarinerConfig resources:
1. cluster1-namespace/submariner-config
2. cluster2-namespace/submariner-config
3. cluster3-namespace/submariner-config

Please select the two SubmarinerConfig CRs for the tunnel you want to fix:
- First SubmarinerConfig (namespace/name):
- Second SubmarinerConfig (namespace/name):
```

**Step 4: Check Current Configuration**

For each selected SubmarinerConfig, check if forceUDPEncaps is already set:
```bash
kubectl get submarinerconfig <name> -n <namespace> -o jsonpath='{.spec.forceUDPEncaps}' --kubeconfig <hub-kubeconfig>
```

Continue to Phase 2 with ACM hub kubeconfig and the two selected SubmarinerConfig references.

### Phase 1-Subctl: For Subctl Deployments

For subctl deployments, you'll work directly with the two cluster kubeconfigs provided.

No additional information needed. Continue to Phase 2.

### Phase 2: Explain What Will Happen

Before applying the fix, explain clearly based on deployment type:

**For ACM Deployments:**
```
This command will test if ESP protocol blocking is the issue by:

1. Applying forceUDPEncaps: true to SubmarinerConfig CRs on the ACM hub
   - Changes will be made on: <namespace1>/submariner-config and <namespace2>/submariner-config

2. Waiting for ACM to propagate changes to managed clusters (10-15 seconds)

3. Gateway pods will automatically restart on both managed clusters

4. Waiting up to 60 seconds for tunnel to establish

5. Reporting if the tunnel comes up

If the tunnel comes up after this change:
✓ ESP blocking was the issue
✓ Keep this setting (recommended)
✓ OR allow ESP (IP protocol 50) in your firewall and remove the setting

If the tunnel still doesn't come up:
⚠ ESP blocking is NOT the issue
⚠ The problem is something else (likely UDP port 4500 blocked)
```

**For Subctl Deployments:**
```
This command will test if ESP protocol blocking is the issue by:

1. Applying ceIPSecForceUDPEncaps: true to Submariner CR on BOTH clusters
   - Changes will be made on both cluster1 and cluster2

2. Restarting gateway pods on both clusters

3. Waiting up to 60 seconds for tunnel to establish

4. Reporting if the tunnel comes up

If the tunnel comes up after this change:
✓ ESP blocking was the issue
✓ Keep this setting (recommended)
✓ OR allow ESP (IP protocol 50) in your firewall and remove the setting

If the tunnel still doesn't come up:
⚠ ESP blocking is NOT the issue
⚠ The problem is something else (likely UDP port 4500 blocked)
```

### Phase 3: Ask for Confirmation

Ask the user:
```
Do you want to proceed with applying UDP encapsulation?
(yes/no)
```

### Phase 4: Apply the Fix

If user confirms, apply the fix based on deployment type:

### Phase 4-ACM: For ACM Deployments

**Step 1: Apply forceUDPEncaps to SubmarinerConfig CRs**

For each of the two selected SubmarinerConfig resources, apply the configuration:

```bash
kubectl patch submarinerconfig <name> -n <namespace> \
  --type merge \
  -p '{"spec":{"forceUDPEncaps":true}}' \
  --kubeconfig <hub-kubeconfig>
```

**Step 2: Wait for Changes to Propagate**

Wait 10-15 seconds for ACM to propagate the configuration changes to the managed clusters.

**Step 3: Verify Gateway Pods Restart**

On each managed cluster, verify that gateway pods are restarting:
```bash
kubectl get pods -n submariner-operator -l app=submariner-gateway --kubeconfig <cluster-kubeconfig>
```

Watch for pods to complete their restart cycle.

**Step 4: Wait for Tunnel Status**

Wait up to 60 seconds and check tunnel status on one of the clusters:
```bash
subctl show connections --kubeconfig <cluster-kubeconfig>
```

Look for STATUS to change from "error" to "connected".

### Phase 4-Subctl: For Subctl Deployments

Use the `submariner_fix_esp_firewall` MCP tool for the two clusters:

Parameters:
- `cluster1_kubeconfig`: Path to first cluster kubeconfig
- `cluster2_kubeconfig`: Path to second cluster kubeconfig
- `namespace`: Submariner namespace (optional)
- `verify_timeout_seconds`: 60 (default)

The tool will automatically:
1. Apply ceIPSecForceUDPEncaps: true to Submariner CR on both clusters
2. Restart gateway pods on both clusters
3. Wait for tunnel status
4. Report results

**Alternatively, manually apply for subctl:**

For each cluster:
```bash
kubectl patch submariner -n submariner-operator \
  --type merge \
  -p '{"spec":{"ceIPSecForceUDPEncaps":true}}' \
  --kubeconfig <cluster-kubeconfig>
```

Then restart gateway pods:
```bash
kubectl delete pods -n submariner-operator -l app=submariner-gateway --kubeconfig <cluster-kubeconfig>
```

### Phase 5: Analyze Results

**OUTCOME 1: Tunnel Connected ✓**
```
✓ SUCCESS: ESP Blocking Was the Issue!

The tunnel came up after forcing UDP encapsulation.

ROOT CAUSE CONFIRMED:
ESP protocol (IP 50) is blocked in your firewall, preventing
Submariner from using native IPSec encapsulation.

SOLUTION APPLIED:
IPSec traffic is now encapsulated in UDP port 4500 instead.

RECOMMENDATION:
Keep this setting permanently. This is a common configuration
for environments where ESP cannot be allowed in firewalls.

Make sure UDP port 4500 is allowed in your firewall rules.

VERIFICATION:
Run /submariner-health to confirm everything is healthy.
```

**OUTCOME 2: Tunnel Still Not Connected ⚠**
```
⚠ ESP Blocking Was NOT the Issue

The tunnel did not come up after forcing UDP encapsulation.

This means the problem is NOT ESP protocol blocking.

NEXT STEPS:
1. Verify UDP port 4500 is allowed in your firewall
2. Check gateway pod logs for new errors:
   kubectl logs -n submariner-operator -l app=submariner-gateway --kubeconfig <cluster-kubeconfig>

3. The issue might be:
   - UDP port 4500 blocked
   - Network connectivity between gateways
   - IPSec configuration mismatch
   - Other firewall rules

RECOMMENDED:
Run /submariner-health again to reassess the situation.
```

### Phase 6: Provide Summary

Always end with:
```
=== ESP CHECK SUMMARY ===

ACTION TAKEN: Applied UDP encapsulation on both clusters
RESULT: [Tunnel connected / Tunnel still not connected]
ROOT CAUSE: [Confirmed ESP blocking / NOT ESP blocking]
NEXT STEP: [Keep setting / Investigate other issues]

FOR MULTI-CLUSTER MESH:
If you have additional clusters (cluster3, cluster4, etc.), you need to:
1. Run this command again for each tunnel pair, OR
2. Manually apply forceUDPEncaps to remaining clusters using kubectl

After fixing all tunnels, verify with: /submariner-health <each-cluster-kubeconfig>
```

## Important Notes

1. **This is a TEST** - we're testing if ESP blocking is the issue
2. **Both clusters required** - the setting must be applied on both sides
3. **For ACM deployments:**
   - First detect ACM by checking for submariner-addon pod
   - Requires ACM hub kubeconfig (not managed cluster kubeconfigs)
   - Sets `forceUDPEncaps: true` in SubmarinerConfig CRs on the hub
   - ACM propagates changes to managed clusters automatically
4. **For subctl deployments:**
   - Sets `ceIPSecForceUDPEncaps: true` in Submariner CR on each cluster
   - Requires kubeconfigs for both managed clusters
5. **UDP port 4500 must be allowed** - if this port is also blocked, the fix won't work
6. **Idempotent** - safe to run multiple times

## What This Command Does NOT Do

- Does NOT fix UDP port blocking (port 4500)
- Does NOT fix network connectivity issues
- Does NOT fix IPSec configuration mismatches
- Does NOT guarantee tunnel will come up (only tests ESP as root cause)

## Success Criteria

The command is successful if:
- ✓ Configuration applied on both clusters
- ✓ Gateway pods restarted successfully
- ✓ Clear determination whether ESP blocking was the issue

Be clear, be direct, and help the user understand what happened!
