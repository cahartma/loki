# Enhanced Schema Status and Warning Conditions

**Branch:** LOG-9351-enhanced-schema-status  
**JIRA:** https://redhat.atlassian.net/browse/LOG-9351  
**Spike Document:** https://docs.google.com/document/d/1EVSyxAipn564KM4TBvTMprfCdu0w2C54JyswZKm_uto/edit?pli=1&tab=t.0  
**Date:** 2026-09-22

## Overview

This feature implements three related enhancements to the Loki Operator schema configuration:

1. **Required Schemas Field** - Makes the `schemas` field required in `ObjectStorageSpec`
2. **Extended Schema Status** - Adds `endDate` and `status` fields to track schema lifecycle
3. **Warning Conditions** - Alerts when schemas need attention (obsolete, old versions)

## Feature 1: Required Schemas Field

### Problem

Prior to Loki 4.0, the operator provided a default schema configuration (v11/BoltDB) when users didn't specify one. However:
- BoltDB is removed in Loki 4.0
- The default was only for backward compatibility
- New deployments should explicitly choose their schema

### Solution

Made `schemas` a required field with OpenAPI validation:

```yaml
spec:
  storage:
    schemas:  # Now required, must have at least 1 schema
    - version: v13
      effectiveDate: "2024-09-01"
```

**Validation Rules:**
- Field is required (cannot be omitted)
- Must contain at least 1 schema (`minItems: 1`)
- Prevents accidental deployments with outdated defaults

## Feature 2: Extended Schema Status

### Problem

Operators needed better visibility into schema lifecycle:
- When does a schema stop being used for writes?
- Is a schema safe to remove from configuration?
- What is the current state of each schema?

### Solution

Enhanced `status.storage.schemas` with calculated fields:

```yaml
status:
  storage:
    schemas:
    - effectiveDate: "2023-01-01"
      version: v12
      endDate: "2024-06-01"    # NEW: When this schema stopped being used for writes
      status: obsolete         # NEW: Schema lifecycle state
    - effectiveDate: "2024-06-01"
      version: v13
      status: inuse            # Currently active schema
```

### Schema Status States

**obsolete** - Schema has fallen out of retention and can be safely removed
```
Condition: current_date >= (endDate + retention_period)
```

**inuse** - Schema is currently active for reading or writing data
```
Condition: effectiveDate <= current_date AND (is latest OR current_date < endDate + retention)
```

**future** - Schema will activate in the future
```
Condition: effectiveDate > current_date
```

### endDate Calculation

The `endDate` field is automatically calculated by the operator:

```
For schema at index i:
  if schemas[i+1] exists:
    endDate = schemas[i+1].effectiveDate
  else:
    endDate = nil  # This is the current/active schema
```

**Example:**
```yaml
spec:
  storage:
    schemas:
    - version: v12
      effectiveDate: "2023-01-01"
    - version: v13
      effectiveDate: "2024-06-01"

# Operator calculates:
status:
  storage:
    schemas:
    - effectiveDate: "2023-01-01"
      version: v12
      endDate: "2024-06-01"     # ← Copied from next schema's effectiveDate
      status: obsolete          # ← Based on retention calculation
    - effectiveDate: "2024-06-01"
      version: v13
      # No endDate - this is the active schema
      status: inuse
```

## Implementation Details

### API Changes

**File:** `api/loki/v1/lokistack_types.go`

Added schema status types:
```go
type ObjectStorageSchemaStatusType string

const (
    SchemaStatusObsolete ObjectStorageSchemaStatusType = "obsolete"
    SchemaStatusInUse    ObjectStorageSchemaStatusType = "inuse"
    SchemaStatusFuture   ObjectStorageSchemaStatusType = "future"
)

type ObjectStorageSchemaStatus struct {
    Version       ObjectStorageSchemaVersion
    EffectiveDate StorageSchemaEffectiveDate
    EndDate       StorageSchemaEffectiveDate         // NEW
    Status        ObjectStorageSchemaStatusType      // NEW
}
```

Made schemas required:
```go
type ObjectStorageSpec struct {
    // +required
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinItems:=1
    Schemas []ObjectStorageSchema `json:"schemas"`
    ...
}
```

### Status Calculation Logic

**File:** `internal/status/storage.go`

```go
func ComputeSchemaStatus(schemas []ObjectStorageSchema, now time.Time, retentionDays int) []ObjectStorageSchemaStatus {
    // Sort schemas by effective date
    // Calculate endDate for each schema (next schema's effectiveDate)
    // Determine status based on:
    //   - Is effectiveDate in the future? → future
    //   - Has endDate + retention passed? → obsolete
    //   - Otherwise → inuse
}
```

**Retention Integration:**
- Uses maximum retention across all tenants and streams
- Checks global retention and per-tenant retention
- Schema marked obsolete only when ALL data has expired

## Feature 3: Warning Conditions

### Problem

Users need proactive alerts when schemas require attention, such as:
- Schemas that can be safely removed (obsolete)
- Schemas using outdated versions
- Future schemas scheduled with old versions

### Solution

Added three new warning conditions that appear in OpenShift Console and can drive alerts:

**1. ObsoleteSchemaPresent**
- **Triggers:** One or more schemas have `status: obsolete`
- **Message:** "One or more schemas are obsolete and no longer in use due to retention settings. These schemas can be safely removed from the configuration."
- **Action:** Remove obsolete schemas from spec

**2. OldSchemaVersion**  
- **Triggers:** Active schemas (status: inuse) are using v11 or v12
- **Message:** "One or more active schemas are using an older schema version. Consider migrating to v13 for better performance."
- **Action:** Plan migration to v13

**3. FutureOldSchemaVersion**
- **Triggers:** Future schemas (status: future) are using v11 or v12
- **Message:** "One or more future schemas are scheduled with an older schema version. Consider using v13 for new schemas."
- **Action:** Update future schemas to v13

These warnings appear in:
- `kubectl get lokistack <name> -o yaml` under `.status.conditions`
- OpenShift Console LokiStack details page
- Can be exposed as Prometheus metrics for alerting

### Changes Summary

**API Types:**
- Added `ObjectStorageSchemaStatusType` enum (obsolete/inuse/future)
- Added `ObjectStorageSchemaStatus` with endDate and status fields
- Made `schemas` field required with validation
- Added 3 new `LokiStackConditionReason` constants for warnings
- Updated v1beta1 conversion tests

**Status Logic:**
- Implemented `ComputeSchemaStatus()` for calculating schema states
- Added `SetStorageSchemaStatus()` for status updates
- Enhanced `checkSchemaWarnings()` to inspect status schemas
- Integrated retention period checking

**Validation:**
- CRD validation prevents empty or missing schemas array
- OpenAPI schema enforces required field at API level

**Generated Artifacts:**
- Regenerated CRD bundles with schema requirements and warning reasons
- Updated API documentation
- Updated sample manifests

## Testing

**Test Results:** See [spike-test-results.md](./spike-test-results.md)

**Tests Passed:**
1. ✅ CRD has schemas in required fields
2. ✅ Creation without schemas rejected with clear error
3. ✅ Creation with schemas succeeds
4. ✅ Status.storage.schemas populated with calculated fields
5. ✅ endDate calculated correctly for superseded schemas
6. ✅ Current schema has no endDate

**Coverage:**
- Unit tests for status calculation logic
- Integration tests with operator deployment
- Validation tests for required field enforcement
- API conversion tests (v1beta1 ↔ v1)

## Usage Examples

### View Schema Status

```bash
kubectl get lokistack my-loki -o jsonpath='{.status.storage.schemas}' | jq
```

Output:
```json
[
  {
    "effectiveDate": "2023-01-01",
    "version": "v12",
    "endDate": "2024-06-01",
    "status": "obsolete"
  },
  {
    "effectiveDate": "2024-06-01",
    "version": "v13",
    "status": "inuse"
  }
]
```

### Check if Schema Can Be Removed

```bash
# Schema can be removed when: current_date >= (endDate + retention_days)
END_DATE=$(kubectl get lokistack my-loki -o jsonpath='{.status.storage.schemas[0].endDate}')
RETENTION=$(kubectl get lokistack my-loki -o jsonpath='{.spec.limits.global.retention.days}')

if [ ! -z "$END_DATE" ]; then
  REMOVAL_DATE=$(date -d "$END_DATE + $RETENTION days" +%Y-%m-%d)
  echo "Schema can be removed after: $REMOVAL_DATE"
fi
```

### Create LokiStack with Explicit Schema

```yaml
apiVersion: loki.grafana.com/v1
kind: LokiStack
metadata:
  name: my-loki
spec:
  size: 1x.small
  storage:
    schemas:  # Required field
    - version: v13
      effectiveDate: "2024-09-01"
    secret:
      name: my-storage-secret
      type: s3
  tenants:
    mode: openshift-logging
```

## Migration Guide

For existing LokiStacks created before this feature:

1. **Check current schema:**
   ```bash
   kubectl get lokistack <name> -o yaml | grep -A5 schemas:
   ```

2. **If schemas field is missing,** add it explicitly:
   ```yaml
   spec:
     storage:
       schemas:
       - version: v13
         effectiveDate: "2024-09-01"  # Use current date
   ```

3. **Monitor status** after update:
   ```bash
   kubectl get lokistack <name> -o jsonpath='{.status.storage.schemas}' | jq
   ```

## Benefits

1. **Prevents Misconfiguration** - Required field ensures users make explicit schema choices
2. **Lifecycle Visibility** - Status fields show when schemas can be safely removed
3. **Retention Awareness** - Status calculation respects retention settings
4. **Future-proof** - Removes dependency on deprecated BoltDB default
5. **Operational Clarity** - Operators can see schema state at a glance

## References

- **JIRA:** https://redhat.atlassian.net/browse/LOG-9351
- **Test Results:** [spike-test-results.md](./spike-test-results.md)
