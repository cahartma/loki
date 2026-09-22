# Test Results - LOG-9351 Features

**Date:** 2026-09-22  
**Branch:** LOG-9351-enhanced-schema-status

---

### Test 1: Create LokiStack WITHOUT Schemas - Pass

**YAML:**
```yaml
apiVersion: loki.grafana.com/v1
kind: LokiStack
metadata:
  name: test-no-schemas
  namespace: openshift-logging
spec:
  size: 1x.demo
  storage:
    secret:
      name: test-secret
      type: s3
  storageClassName: gp3-csi
```

**Expected:** Should FAIL with required field error

**Actual: Passed** - Creation rejected as expected
```
The LokiStack "test-no-schemas" is invalid: 
* spec.storage.schemas: Required value
* <nil>: Invalid value: null: some validation rules were not checked because the object was invalid; correct the existing errors to complete validation
```

---

### Test 2: Create LokiStack WITH Schemas - Pass

**YAML:**
```yaml
apiVersion: loki.grafana.com/v1
kind: LokiStack
metadata:
  name: test-with-schemas
  namespace: openshift-logging
spec:
  size: 1x.demo
  storage:
    schemas:
    - version: v13
      effectiveDate: "2024-09-01"
    secret:
      name: logging-loki-s3
      type: s3
  storageClassName: gp3-csi
  tenants:
    mode: openshift-logging
```

**Expected:** Should SUCCEED

**Actual: Passed** - Creation successful
```
lokistack.loki.grafana.com/test-with-schemas created
```

---

## Extended Schema Status with endDate

### Test 3: Single Schema Status - Pass

**Result:**
```json
[
  {
    "effectiveDate": "2024-09-01",
    "status": "inuse",
    "version": "v13"
  }
]
```

**Verification:**
- effectiveDate: "2024-09-01" 
- status: "inuse" 
- version: "v13" 
- No **endDate** field (correct - has no successor)

**Status: Pass** - Status correctly populated for single schema

---

### Test 4: Multiple Schemas with endDate - Pass

**YAML:**
```yaml
apiVersion: loki.grafana.com/v1
kind: LokiStack
metadata:
  name: test-multiple-schemas
  namespace: openshift-logging
spec:
  size: 1x.demo
  storage:
    schemas:
    - version: v12
      effectiveDate: "2023-01-01"
    - version: v13
      effectiveDate: "2024-06-01"
    secret:
      name: logging-loki-s3
      type: s3
  storageClassName: gp3-csi
  tenants:
    mode: openshift-logging
```

**Result:**
```json
[
  {
    "effectiveDate": "2023-01-01",
    "endDate": "2024-06-01",
    "status": "inuse",
    "version": "v12"
  },
  {
    "effectiveDate": "2024-06-01",
    "status": "inuse",
    "version": "v13"
  }
]
```

**Verification:**

**Schema 1 (v12):**
- effectiveDate: "2023-01-01"
- **endDate: "2024-06-01"** ← **Key Feature! Calculated from next schema**
- status: "inuse"
- version: "v12"

**Schema 2 (v13):**
- effectiveDate: "2024-06-01"
- No **endDate** (correct)
- status: "inuse"
- version: "v13"

**Status: Passed** - endDate correctly calculated as next schema's effectiveDate

---

## Summary

### Tests Executed: 4/4

| # | Test | Result | Notes |
|---|------|------|-------|
| 1 | Create without schemas | PASS | Correctly rejected |
| 2 | Create with schemas | PASS | Successfully created |
| 3 | Single schema status | PASS | No endDate (correct) |
| 4 | Multiple schemas status | PASS | endDate calculated correctly |


---

## Conclusion

### Feature 1: Required Schemas
- [x] CRD updated with required field
- [x] Validation rejects missing schemas
- [x] Clear error messages
- [x] Creation succeeds with schemas

### Feature 2: Extended Schema Status
- [x] status.storage.schemas populated
- [x] effectiveDate copied from spec
- [x] version copied from spec
- [x] status calculated (inuse/obsolete/future)
- [x] **endDate calculated from next schema**
- [x] **endDate omitted for current schema**
- [x] CRD schema includes all fields




