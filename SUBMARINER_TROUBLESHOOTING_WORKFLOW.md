# Submariner Troubleshooting Workflow Guide

This document describes the structured troubleshooting workflow for Submariner using the kubernetes-mcp-server slash commands.

## Quick Start

**Always start here:**
```
/submariner-health
```

This is your entry point. It will scan your deployment and tell you exactly what to do next.

---

## The Complete Workflow

```
┌─────────────────────────┐
│  /submariner-health     │  ← START HERE
│  (Health Check)         │
└───────────┬─────────────┘
            │
            ├─ Tunnel NOT Connected ──────────┐
            │                                  │
            ├─ Tunnel Connected ───────────────┼─ ✓ All Good
            │                                  │
            └─ ESP Issue Detected ─────────────┤
                                               │
                                               ▼
                          ┌─────────────────────────────────┐
                          │ /submariner-tunnel-troubleshoot │
                          │  (Investigate Root Cause)       │
                          └───────────┬─────────────────────┘
                                      │
                    ┌─────────────────┼──────────────────┐
                    │                 │                  │
                    ▼                 ▼                  ▼
           ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
           │ ESP Blocking │  │ UDP Port     │  │  Network     │
           │   Detected   │  │  Blocking    │  │ Connectivity │
           └──────┬───────┘  └──────┬───────┘  └──────┬───────┘
                  │                 │                  │
                  ▼                 ▼                  ▼
         ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
         │/submariner-  │  │   Manual     │  │   Manual     │
         │tunnel-esp-   │  │  Firewall    │  │   Network    │
         │check         │  │     Fix      │  │Troubleshooting│
         └──────┬───────┘  └──────────────┘  └──────────────┘
                │
                ├─ Tunnel UP ───────────┐
                │                       │
                └─ Still Down ──────────┤
                                        │
                                        ▼
                            ┌────────────────────┐
                            │ /submariner-health │
                            │  (Re-assess)       │
                            └────────────────────┘
```

---

## Step-by-Step Guide

### Step 1: Initial Health Check

**Command:**
```
/submariner-health
```

**What it does:**
- ✓ Checks all Submariner pod status
- ✓ Examines inter-cluster tunnel status (CRITICAL)
- ✓ Analyzes pod logs for errors
- ✓ Runs subctl show and diagnose
- ✓ Detects ESP/firewall issues automatically
- ✓ **Recommends the exact next command to run**

**Expected Output:**
```
=== HEALTH CHECK SUMMARY ===

✓ Pod Status: All healthy
⚠ Tunnel Status: NOT CONNECTED
✓ Logs: No errors
⚠ ESP/Firewall: Potential ESP blocking detected
✓ Diagnostics: All passed

CURRENT STATE: Tunnel not connected, using private IP
NEXT COMMAND: /submariner-tunnel-troubleshoot
REASON: Investigate why tunnel is not establishing
```

---

### Step 2: Tunnel Troubleshooting (If Tunnel Not Connected)

**Command:**
```
/submariner-tunnel-troubleshoot
```

**What it does:**
- Analyzes gateway pod logs on **both clusters**
- Checks cable driver type (IPSec vs WireGuard)
- Examines IP selection (private vs public)
- Identifies error patterns in logs
- Diagnoses the root cause
- **Recommends specific fix command**

**Expected Output:**
```
=== TUNNEL TROUBLESHOOTING SUMMARY ===

FINDING: Using private IP with IPSec, ESP errors in gateway logs
ROOT CAUSE: ESP protocol (IP 50) blocked in firewall
NEXT COMMAND: /submariner-tunnel-esp-check
EXPECTED OUTCOME: Tunnel should come up with UDP encapsulation
```

---

### Step 3: ESP Firewall Check and Fix

**Command:**
```
/submariner-tunnel-esp-check
```

**What it does:**
- Explains what will happen before making changes
- Asks for user confirmation
- Applies UDP encapsulation on **both clusters**
  - ACM: Sets `forceUDPEncaps: true` in SubmarinerConfig
  - subctl: Sets `ceIPSecForceUDPEncaps: true` in Submariner CR
- Restarts gateway pods
- Waits for tunnel to establish
- **Determines if ESP blocking was the root cause**

**Expected Output (Success):**
```
✓ SUCCESS: ESP Blocking Was the Issue!

The tunnel came up after forcing UDP encapsulation.

ROOT CAUSE CONFIRMED:
ESP protocol (IP 50) is blocked in your firewall.

SOLUTION APPLIED:
IPSec traffic now uses UDP port 4500 instead.

RECOMMENDATION:
Keep this setting permanently.

VERIFICATION:
Run /submariner-health to confirm everything is healthy.
```

**Expected Output (Still Failed):**
```
⚠ ESP Blocking Was NOT the Issue

The tunnel did not come up after forcing UDP encapsulation.

NEXT STEPS:
1. Verify UDP port 4500 is allowed in your firewall
2. Check gateway pod logs for new errors
3. Issue might be UDP port blocking or network connectivity

RECOMMENDED:
Run /submariner-health again to reassess.
```

---

### Step 4: Verification (Return to Health Check)

**Command:**
```
/submariner-health
```

**What it does:**
- Re-runs all health checks
- Confirms tunnel is now connected
- Verifies no new issues appeared

**Expected Output (If Fixed):**
```
=== HEALTH CHECK SUMMARY ===

✓ Pod Status: All healthy
✓ Tunnel Status: Connected
✓ Logs: No errors
✓ ESP/Firewall: Using UDP encapsulation
✓ Diagnostics: All passed

✓ ALL CHECKS PASSED

Your Submariner deployment is healthy!
```

---

## Common Scenarios

### Scenario 1: Fresh Deployment Not Connecting

```
User: /submariner-health
    ↓ Output: Tunnel NOT CONNECTED, ESP issue detected

User: /submariner-tunnel-troubleshoot
    ↓ Output: Root cause = ESP blocking

User: /submariner-tunnel-esp-check
    ↓ Output: ✓ Tunnel came up!

User: /submariner-health
    ↓ Output: ✓ All healthy
```

**Total time:** ~5 minutes

---

### Scenario 2: Was Working, Now Broken

```
User: /submariner-health
    ↓ Output: Tunnel error, recent errors in gateway logs

User: /submariner-tunnel-troubleshoot
    ↓ Output: Authentication errors in logs
    ↓ Recommendation: Check IPSec PSK or certificates

User: [Manually verify broker configuration]
    ↓ Fix: Redeploy or fix PSK mismatch

User: /submariner-health
    ↓ Output: ✓ All healthy
```

---

### Scenario 3: Intermittent Connectivity

```
User: /submariner-health
    ↓ Output: Tunnel connected, but datapath tests needed

User: /submariner-mtu-fix
    ↓ Tests large packets (~3000 bytes)
    ↓ Output: MTU issue detected
    ↓ Applies TCP MSS clamping

User: /submariner-health
    ↓ Output: ✓ All healthy
```

---

## Command Reference

| Command | Purpose | When to Use |
|---------|---------|-------------|
| `/submariner-health` | Entry point & verification | Always start here |
| `/submariner-tunnel-troubleshoot` | Diagnose tunnel issues | Tunnel not connected |
| `/submariner-tunnel-esp-check` | Test/fix ESP blocking | ESP identified as root cause |
| `/submariner-mtu-fix` | Fix MTU issues | Large packets fail |
| `/submariner-troubleshoot` | Complete guided workflow | Unsure what's wrong |

---

## Decision Tree

**Is your tunnel connected?**
- ❌ **NO** → Run `/submariner-tunnel-troubleshoot`
  - Diagnoses: ESP? UDP port? Network?
  - Follow recommended command

- ✓ **YES** → Run optional verification
  - `/submariner-mtu-fix` (test large packets)
  - Or you're done! ✓

**Did ESP check fix it?**
- ✓ **YES** → Keep UDP encapsulation setting
  - ESP was blocked
  - UDP port 4500 is now used
  - Verify with `/submariner-health`

- ❌ **NO** → Issue is NOT ESP
  - Check UDP port 4500 firewall rules
  - Verify network connectivity
  - Check IPSec configuration
  - Re-run `/submariner-health`

---

## Key Principles

1. **Always start with `/submariner-health`**
   - It's the compass that points you in the right direction

2. **Follow the recommended next command**
   - Each command tells you what to do next
   - Don't skip steps or guess

3. **One issue at a time**
   - Fix the most critical issue first (usually tunnel connectivity)
   - Re-assess after each fix

4. **Verify after fixes**
   - Always run `/submariner-health` again after applying a fix
   - Confirm the issue is resolved

5. **Both clusters matter**
   - Most fixes require applying changes to BOTH clusters
   - Check logs on BOTH sides

---

## Troubleshooting Tips

### The tunnel is not connecting

**Most common cause (80%):** ESP protocol blocked in firewall
- **Solution:** Run `/submariner-tunnel-esp-check`
- **Result:** Forces UDP encapsulation

### ESP check didn't help

**Next likely cause:** UDP port 4500 blocked
- **Check:** Firewall rules for UDP 500 and 4500
- **Solution:** Open ports in firewall, restart gateway pods

### Pods are crashing

**Check:** Gateway pod logs
```bash
kubectl logs -n submariner-operator -l app=submariner-gateway --tail=100
```
- Look for specific error messages
- Follow troubleshooting based on errors

### Everything looks healthy but datapath fails

**Likely cause:** MTU issues
- **Solution:** Run `/submariner-mtu-fix`
- **Result:** Applies TCP MSS clamping

---

## Quick Recovery Commands

If you want to restart from scratch:

```bash
# Restart all Submariner gateway pods
kubectl delete pods -n submariner-operator -l app=submariner-gateway

# Wait 30 seconds, then check health
sleep 30
```

Then run: `/submariner-health`

---

## Need Help?

If the structured workflow doesn't solve your issue:

1. Run `/submariner-troubleshoot` for a comprehensive guided session
2. Collect logs from both clusters:
   ```bash
   kubectl logs -n submariner-operator -l app=submariner-gateway --tail=500
   ```
3. Run `subctl diagnose all` manually on both clusters
4. Check Submariner documentation: https://submariner.io

---

## Summary

```
START → /submariner-health
           ↓ (Recommends next step)
        /submariner-tunnel-troubleshoot
           ↓ (Diagnoses root cause)
        /submariner-tunnel-esp-check (or other fix)
           ↓ (Applies fix & verifies)
        /submariner-health (Confirm fixed)
           ↓
        ✓ DONE!
```

**The workflow is designed to be:**
- ✓ Simple: Just follow the recommended commands
- ✓ Fast: Most issues resolved in 5-10 minutes
- ✓ Accurate: Automated detection of root causes
- ✓ Complete: Fixes are verified automatically

**Your job:** Run the commands and follow the guidance. The system does the rest!
