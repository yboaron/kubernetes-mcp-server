# Submariner Datapath Verification

You are verifying end-to-end datapath connectivity in Submariner multi-cluster deployments.

## Your Role

The `/submariner-health` command confirmed that the tunnel is **connected**. Now you need to verify that the datapath actually works end-to-end.

This command tests:
- Pod-to-pod connectivity across clusters
- Service connectivity across clusters
- Actual application traffic flow

## Your Task

### Phase 1: Get Cluster Information

**Check if kubeconfigs were provided as parameters:**
- This command can be invoked as `/submariner-datapath-check <kubeconfig1> <kubeconfig2>`
- If both kubeconfig paths are provided as parameters, use them directly
- If not provided, ask the user for them

Ask the user for (if not provided as parameters):
- Kubeconfig path for **cluster 1** (source cluster)
- Kubeconfig path for **cluster 2** (destination cluster)
- Whether they want verbose output (optional)

### Phase 2: Setup KUBECONFIG and Identify Context Names

`subctl verify` requires using contexts, not kubeconfig file paths. Follow these steps:

1. **Set KUBECONFIG environment variable** with both cluster configs:
```bash
export KUBECONFIG=<kubeconfig1>:<kubeconfig2>
```

2. **Identify the context names**:
```bash
kubectl config get-contexts
```

The output will show context names (typically `cluster1`, `cluster2`, `kind-cluster1`, etc.).
Note the context names for both clusters.

### Phase 3: Clean Up Previous Test Artifacts (Optional but Recommended)

Before running verification, clean up any leftover test namespaces from previous runs:

```bash
# Use the context names identified in Phase 2
kubectl --context <context1> get namespaces | grep e2e-tests | awk '{print $1}' | \
  xargs -I {} kubectl --context <context1> delete namespace {} --timeout=30s 2>/dev/null || true

kubectl --context <context2> get namespaces | grep e2e-tests | awk '{print $1}' | \
  xargs -I {} kubectl --context <context2> delete namespace {} --timeout=30s 2>/dev/null || true
```

Wait a few seconds for cleanup to complete.

### Phase 4: Run Datapath Verification

Run `subctl verify` using contexts (this is the ONLY supported method):

```bash
# Ensure KUBECONFIG is set (from Phase 2)
export KUBECONFIG=<kubeconfig1>:<kubeconfig2>

# Run verification using context names
subctl verify \
  --context <context1> \
  --tocontext <context2> \
  --only connectivity \
  --verbose (if user requested verbose output)
```

**CRITICAL**: `subctl verify` ONLY supports `--context` and `--tocontext` flags.
Do NOT use `--kubeconfig` or `--toconfig` - they are not supported.

This will run `subctl verify` which tests:
1. **Pod-to-Pod connectivity**: Deploy test pods and verify they can reach each other
2. **Service connectivity**: Test cross-cluster service access
3. **Globalnet connectivity**: Verify globalnet IP translation works

**Note**: If verbose output was NOT requested, omit the `--verbose` flag.

### Phase 5: Analyze Results

**OUTCOME 1: All Tests Pass ✓**
```
✓ SUCCESS: Datapath is Working!

All connectivity tests passed:
- Pod-to-pod connectivity: ✓
- Service connectivity: ✓
- DNS resolution: ✓

Your Submariner deployment is fully functional!

CURRENT STATE: Healthy and operational
NEXT STEP: No action needed

You can now use cross-cluster services in your applications.
```

**OUTCOME 2: Tests Fail with Default Packets, Pass with Small Packets ⚠**
```
⚠ MTU ISSUE DETECTED

The datapath verification shows:
- Small packets (200 bytes): PASS ✓
- Default packets (~3000 bytes): FAIL ✗

This is a classic MTU/fragmentation issue.

ROOT CAUSE:
Submariner adds IPSec encapsulation overhead (~60-80 bytes) which
reduces the effective MTU. Large packets exceed the path MTU and
are fragmented or dropped.

RECOMMENDED NEXT STEP:
Run: /submariner-mtu-check <kubeconfig1> <kubeconfig2>

This command will:
- Apply TCP MSS clamping to fix the MTU issue
- Test with different packet sizes
- Verify the fix works
```

**OUTCOME 3: All Tests Fail (Even Small Packets) ✗**
```
✗ DATAPATH VERIFICATION FAILED

Both small and large packets are failing.

This indicates a more fundamental issue, NOT MTU:

POSSIBLE CAUSES:
1. Routing issues between clusters
2. Firewall blocking application traffic (not just IPSec)
3. Network policy blocking cross-cluster traffic
4. CNI plugin issues

RECOMMENDED NEXT STEP:
1. Re-run health check to verify tunnel is still connected:
   /submariner-health <kubeconfig>

2. Check gateway logs for new errors:
   kubectl logs -n submariner-operator -l app=submariner-gateway --kubeconfig <kubeconfig>

3. Verify network policies are not blocking traffic

4. Check route agent logs:
   kubectl logs -n submariner-operator -l app=submariner-routeagent --kubeconfig <kubeconfig>
```

**OUTCOME 4: Test Framework Errors (Namespace Conflicts) ⚠**
```
⚠ SUBCTL VERIFY FRAMEWORK ISSUE

The subctl verify tests failed with namespace conflicts:
"namespaces 'e2e-tests-*' already exists"

This is a known bug in the Submariner e2e test framework, NOT a datapath issue.

WORKAROUND - Use Health Check as Verification:

The /submariner-health command already validates datapath connectivity:
- Health check pings use the SAME datapath as pod-to-pod traffic
- If health check shows "connected" with successful pings, datapath is working

Check the health status from /submariner-health:
✓ Tunnel Status: connected
✓ Health Check IP: reachable (with RTT measurement)
✓ Gateway Status: all connections established

If all health checks pass → Datapath is WORKING ✓

RECOMMENDED NEXT STEP:
Option 1: Accept health check results (recommended)
- If /submariner-health shows tunnel connected with successful pings
- Datapath is working, no further action needed

Option 2: Retry after cleanup (if you need subctl verify to pass)
- Wait 60 seconds for namespace finalizers to complete
- Run the cleanup commands again
- Retry subctl verify

The test framework bug does not indicate a real connectivity problem.
If health checks pass, your Submariner deployment is fully functional.
```

### Phase 6: Provide Clear Summary

Always end with:
```
=== DATAPATH VERIFICATION SUMMARY ===

TEST RESULTS:
- Pod connectivity: [PASS/FAIL]
- Service connectivity: [PASS/FAIL]
- DNS resolution: [PASS/FAIL]

DIAGNOSIS: [MTU issue / Working / Other issue]
NEXT COMMAND: /[recommended-command or "none"]
REASON: [Why this command will help]
```

## Important Notes

1. **Tunnel must be connected first** - if tunnel is not connected, this test will fail
2. **Bidirectional testing** - tests run in both directions (cluster1→cluster2 AND cluster2→cluster1)
3. **MTU is the most common issue** when tunnel is up but datapath fails
4. **Service discovery** requires Lighthouse to be installed (optional component)
5. **Test framework bugs**: If you see "namespaces 'e2e-tests-*' already exists" errors, this is OUTCOME 4 (framework bug, not a real issue)
6. **Health check is often sufficient**: The health check from `/submariner-health` already validates the datapath

## Detecting Test Framework Issues

If the `subctl verify` output shows:
- Multiple test failures (10+ failures)
- ALL failures have the same error message
- Error message contains: "namespaces ... already exists" or "AlreadyExists"
- Error occurs in `[BeforeEach]` phase (before actual test logic runs)

→ This is **OUTCOME 4** (test framework bug, not a connectivity issue)

## Test Flow

```
Phase 1: Get kubeconfig paths from user
  ↓
Phase 2: Export KUBECONFIG=path1:path2 and identify context names
  ↓
Phase 3: Clean up old test namespaces (optional but recommended)
  ↓
Phase 4: Run subctl verify --context ctx1 --tocontext ctx2 --only connectivity
  ↓
Phase 5: Check results:
  ↓
All Pass? → OUTCOME 1: Success! ✓
  ↓
Framework errors (namespace conflicts)? → OUTCOME 4: Check health status instead
  ↓
Fail with large packets only? → OUTCOME 2: MTU Issue → /submariner-mtu-check
  ↓
All fail (even small packets)? → OUTCOME 3: Deeper investigation needed
  ↓
Phase 6: Provide summary with next steps
```

## Priority Order for Outcomes

1. **Framework errors (namespace conflicts)** → OUTCOME 4 → Rely on health check results
2. **All tests pass** → OUTCOME 1 → Success, no action needed
3. **Large packet failures** → OUTCOME 2 → MTU issue, run /submariner-mtu-check
4. **All tests fail** → OUTCOME 3 → Investigate routing/firewall/network policies

## What This Command Does NOT Do

- Does NOT fix MTU issues (use `/submariner-mtu-check` for that)
- Does NOT troubleshoot tunnel connectivity (use `/submariner-tunnel-troubleshoot` for that)
- Does NOT check tunnel status (use `/submariner-health` for that)
- Does NOT involve manually creating test pods (only use `subctl verify` for datapath testing)

## IMPORTANT: Testing Methodology

**ALWAYS use `subctl verify` for datapath verification.**

Do NOT manually create test pods and run connectivity tests yourself. The proper way to verify the datapath is:
1. Export KUBECONFIG with both cluster configs: `export KUBECONFIG=path1:path2`
2. Use `subctl verify --context <ctx1> --tocontext <ctx2> --only connectivity`
3. If `subctl verify` has framework issues, rely on the health check results from `/submariner-health`
4. The health check pings already validate the same datapath that pods use

**Critical Requirements:**
- `subctl verify` ONLY supports `--context` and `--tocontext` (context names)
- `subctl verify` does NOT support `--kubeconfig` or `--toconfig` flags
- You MUST set `KUBECONFIG` environment variable before running `subctl verify`
- Manual pod creation and connectivity testing should NEVER be used as a substitute

Be clear, be thorough, and guide users to the right next step!
