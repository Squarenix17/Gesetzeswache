package sync

import (
	"context"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/discovery"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/metrics"
)

const metaKeyBGBlFeedDegraded = "last_bgbl_feed_degraded"

func (o *Orchestrator) markBGBlFeedDegraded(at time.Time) {
	_ = o.Store.SetMetaTime(metaKeyBGBlFeedDegraded, at)
}

func (o *Orchestrator) clearBGBlFeedDegraded() {
	_ = o.Store.SetMeta(metaKeyBGBlFeedDegraded, "")
}

func (o *Orchestrator) stampSuccessMeta(key string, t time.Time) error {
	existing, ok, err := o.Store.GetMetaTime(key)
	if err != nil {
		return err
	}
	if ok && existing.After(t) {
		if o.Log != nil {
			o.Log.Warn("clock jump: not moving success timestamp backwards", "key", key, "stored", existing, "new", t)
		}
		return nil
	}
	return o.Store.SetMetaTime(key, t)
}

func firstStoreErr(current, err error) error {
	if current != nil || err == nil {
		return current
	}
	return err
}

func (o *Orchestrator) recordStoreWriteFailure(source string) {
	if o.Metrics == nil {
		return
	}
	_ = o.Metrics.IncCounter(metrics.MetricSyncStoreWriteFailuresTotal, map[string]string{
		"source": source,
	}, 1)
}

func (o *Orchestrator) recordStandRefreshFailure() {
	if o.Metrics == nil {
		return
	}
	_ = o.Metrics.IncCounter(metrics.MetricStandRefreshFailuresTotal, nil, 1)
}

func (o *Orchestrator) recordDiscoveryIngest(result string) {
	if o.Metrics == nil {
		return
	}
	_ = o.Metrics.IncCounter(metrics.MetricDiscoveryIngestTotal, map[string]string{
		"result": result,
	}, 1)
}

func (o *Orchestrator) discoveryIngestLaw(ctx context.Context, law domain.Law, lookup discovery.CatalogLookup) (bool, error) {
	xmlData, err := o.fetchLawXML(ctx, law)
	if err != nil {
		return false, err
	}
	if _, err := discovery.IngestLawXML(o.Store, lookup, law, xmlData); err != nil {
		return false, err
	}
	return true, nil
}
