package status

import (
	"context"
	"sort"
	"time"

	"github.com/ViaQ/logerr/v2/kverrors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	lokiv1 "github.com/grafana/loki/operator/api/loki/v1"
	"github.com/grafana/loki/operator/internal/external/k8s"
)

// SetStorageSchemaStatus updates the storage status component
func SetStorageSchemaStatus(ctx context.Context, k k8s.Client, req ctrl.Request, schemas []lokiv1.ObjectStorageSchema) error {
	var s lokiv1.LokiStack
	if err := k.Get(ctx, req.NamespacedName, &s); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return kverrors.Wrap(err, "failed to lookup lokistack", "name", req.NamespacedName)
	}

	// Calculate schema status with retention information
	var retentionDays int
	if s.Spec.Limits != nil {
		retentionDays = getMaxRetentionDays(s.Spec.Limits)
	}

	s.Status.Storage.Schemas = ComputeSchemaStatus(schemas, time.Now().UTC(), retentionDays)
	return k.Status().Update(ctx, &s)
}

// ComputeSchemaStatus transforms schemas into status objects with calculated state
func ComputeSchemaStatus(schemas []lokiv1.ObjectStorageSchema, now time.Time, retentionDays int) []lokiv1.ObjectStorageSchemaStatus {
	if len(schemas) == 0 {
		return nil
	}

	// Sort schemas by effective date
	sorted := make([]lokiv1.ObjectStorageSchema, len(schemas))
	copy(sorted, schemas)
	sort.Slice(sorted, func(i, j int) bool {
		di, _ := sorted[i].EffectiveDate.UTCTime()
		dj, _ := sorted[j].EffectiveDate.UTCTime()
		return di.Before(dj)
	})

	result := make([]lokiv1.ObjectStorageSchemaStatus, 0, len(sorted))

	// Calculate retention cutoff date (schemas older than this are obsolete)
	var retentionCutoff time.Time
	if retentionDays > 0 {
		retentionCutoff = now.AddDate(0, 0, -retentionDays)
	}

	for i, schema := range sorted {
		effectiveDate, _ := schema.EffectiveDate.UTCTime()

		status := lokiv1.ObjectStorageSchemaStatus{
			Version:       schema.Version,
			EffectiveDate: schema.EffectiveDate,
		}

		// Determine end date (effective date of next schema, or empty for last)
		if i < len(sorted)-1 {
			status.EndDate = sorted[i+1].EffectiveDate
		}

		// Determine status: obsolete, inuse, or future
		if effectiveDate.After(now) {
			// Schema becomes active in the future
			status.Status = lokiv1.SchemaStatusFuture
		} else if retentionDays > 0 && !status.EndDate.IsZero() {
			// Check if schema is obsolete (older than retention)
			endDate, _ := status.EndDate.UTCTime()
			if endDate.Before(retentionCutoff) {
				status.Status = lokiv1.SchemaStatusObsolete
			} else {
				status.Status = lokiv1.SchemaStatusInUse
			}
		} else {
			// Schema is currently active or was active (no retention or is last schema)
			status.Status = lokiv1.SchemaStatusInUse
		}

		result = append(result, status)
	}

	return result
}

// getMaxRetentionDays returns the maximum retention across all tenants
func getMaxRetentionDays(limits *lokiv1.LimitsSpec) int {
	if limits == nil {
		return 0
	}

	maxDays := 0

	// Check global retention
	if limits.Global != nil && limits.Global.Retention != nil {
		globalDays := int(limits.Global.Retention.Days)
		if globalDays > maxDays {
			maxDays = globalDays
		}

		// Check per-stream overrides in global
		for _, stream := range limits.Global.Retention.Streams {
			if stream != nil && int(stream.Days) > maxDays {
				maxDays = int(stream.Days)
			}
		}
	}

	// Check per-tenant retention
	for _, tenant := range limits.Tenants {
		if tenant.Retention != nil {
			tenantDays := int(tenant.Retention.Days)
			if tenantDays > maxDays {
				maxDays = tenantDays
			}

			// Check per-stream overrides for tenant
			for _, stream := range tenant.Retention.Streams {
				if stream != nil && int(stream.Days) > maxDays {
					maxDays = int(stream.Days)
				}
			}
		}
	}

	return maxDays
}
