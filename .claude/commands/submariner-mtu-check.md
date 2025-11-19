# Submariner MTU Issue Check and Fix

You are helping troubleshoot and fix MTU (Maximum Transmission Unit) issues in Submariner multi-cluster connectivity.

## Background

Submariner adds IPSec encapsulation overhead (~60-80 bytes) which reduces the effective MTU. Submariner's datapath e2e tests use ~3000 byte packets to verify connectivity. When these large packets exceed the path MTU, they are fragmented or dropped, causing connectivity issues. This is commonly seen when:
- Small packets (200 bytes) work fine
- Large packets (~3000 bytes) fail or timeout
- TCP connections hang or reset
- Datapath verification tests fail

**Important:** Submariner datapath tests are **bidirectional** (cluster1→cluster2 AND cluster2→cluster1). This means:
- An MTU issue in **only one cluster** can cause the entire datapath test to fail
- Even if one direction works, the return path may fail due to MTU issues
- You may need to apply the fix to only the affected cluster, not necessarily both
- However, it's often safer to apply the fix to both clusters for consistency

## Your Task

Guide the user through detecting and fixing MTU issues:

1. **Get cluster information:**

   **Check if kubeconfigs were provided as parameters:**
   - This command can be invoked as `/submariner-mtu-check <kubeconfig1> <kubeconfig2>`
   - If both kubeconfig paths are provided as parameters, use them directly
   - If not provided, ask the user for them

   Ask the user for (if not provided as parameters):
   - Kubeconfig path for **source cluster**
   - Kubeconfig path for **destination cluster**
   - Whether they want to specify custom contexts

2. **Run datapath verification:**
   - Use the `submariner_verify_datapath` MCP tool with:
     - `from_kubeconfig`: Source cluster kubeconfig
     - `to_kubeconfig`: Destination cluster kubeconfig
     - `auto_troubleshoot`: true (this will automatically test with small packets)
     - `verbose`: false (unless user wants detailed output)

3. **Analyze the results:**
   - If tests pass with small packets (200 bytes) but fail with default size: MTU issue confirmed!
   - **Important**: The datapath test only reports that TCP connectivity failed - it does NOT indicate which direction (cluster1→cluster2 or cluster2→cluster1) has the MTU issue
   - **This means**:
     - The MTU issue could be in cluster1's outgoing path
     - OR it could be in cluster2's outgoing path
     - OR both clusters could have MTU issues
   - **Recommendation**: Since you cannot determine which cluster has the issue from the test output alone, the safest approach is to apply the TCP MSS clamping fix to **both clusters**
   - Explain the diagnosis to the user

4. **Apply TCP MSS clamping fix:**
   - Explain what TCP MSS clamping is and why it helps
   - **Since the datapath test doesn't tell us which cluster has the MTU issue, apply the fix to BOTH clusters**
   - This ensures the issue is resolved regardless of which direction is affected

   Guide the user to apply the fix to both clusters:
     - Identify the gateway node(s)
     - Annotate with `submariner.io/tcp-clamp-mss=1200`
     - Restart RouteAgent pods

   Provide the exact commands (repeat for BOTH clusters):
   ```bash
   # === FOR CLUSTER 1 ===
   # Find gateway nodes
   kubectl get nodes -l submariner.io/gateway=true --kubeconfig <cluster1-kubeconfig>

   # Annotate gateway node with TCP MSS clamping
   kubectl annotate node <gateway-node-name> submariner.io/tcp-clamp-mss=1200 --kubeconfig <cluster1-kubeconfig>

   # Restart RouteAgent pods
   kubectl delete pod -n submariner-operator -l app=submariner-routeagent --kubeconfig <cluster1-kubeconfig>

   # === FOR CLUSTER 2 ===
   # Find gateway nodes
   kubectl get nodes -l submariner.io/gateway=true --kubeconfig <cluster2-kubeconfig>

   # Annotate gateway node with TCP MSS clamping
   kubectl annotate node <gateway-node-name> submariner.io/tcp-clamp-mss=1200 --kubeconfig <cluster2-kubeconfig>

   # Restart RouteAgent pods
   kubectl delete pod -n submariner-operator -l app=submariner-routeagent --kubeconfig <cluster2-kubeconfig>
   ```

5. **Verify the fix:**
   - Run the datapath verification again using `/submariner-datapath-check`
   - Check if connectivity is now working
   - Suggest adjusting the MSS value if needed (typical range: 1200-1400)

6. **Apply to additional clusters (if multi-cluster mesh):**
   - If you have more than 2 clusters in your clusterset, apply the fix to ALL clusters for consistency
   - Explain to the user: "Since we cannot determine which specific cluster has the MTU issue, we're applying the fix to both clusters being tested. For a multi-cluster mesh, it's recommended to apply this to all clusters."

## Important Notes

- The annotation must be applied to ALL gateway nodes in each cluster
- The MSS value might need tuning based on the network (start with 1200)
- **Bidirectional testing limitation**: Datapath tests run in both directions but only report overall success/failure - they do NOT indicate which specific direction failed
- **Cannot isolate affected cluster**: Since the test doesn't provide directional information, you cannot determine if cluster1→cluster2 or cluster2→cluster1 has the issue
- **Best practice**: Always apply TCP MSS clamping to BOTH clusters being tested (and ideally all clusters in the mesh)
- RouteAgent pods must be restarted for the change to take effect

Be patient, provide clear step-by-step instructions, and help verify the fix worked!
