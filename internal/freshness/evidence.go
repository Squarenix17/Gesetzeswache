package freshness

import "time"

// FutureTimestampTolerance rejects success timestamps more than this far in the future.
const FutureTimestampTolerance = 5 * time.Minute

// BGBLEvidence picks the BGBl evidence timestamp for freshness evaluation.
//
// Rules:
//   - if BGBl feed success is within maxAge → (bgblTime, probeOnly=false)
//   - else if ELI probe success within maxAge AND eliTime.After(bgblTime) → (eliTime, probeOnly=true)
//   - else → (zero, probeOnly=false) meaning no usable evidence
func BGBLEvidence(bgblTime, eliTime, now time.Time, maxAge time.Duration) (time.Time, bool) {
	if maxAge <= 0 {
		maxAge = 6 * time.Hour
	}
	if timestampFresh(bgblTime, now, maxAge) {
		return bgblTime, false
	}
	if timestampFresh(eliTime, now, maxAge) && eliTime.After(bgblTime) {
		return eliTime, true
	}
	return time.Time{}, false
}

// EffectiveBGBlFeedTime returns zero when degradedAt is newer than feedSuccess,
// indicating a partial BGBl feed failure invalidated the last success stamp.
func EffectiveBGBlFeedTime(feedSuccess, degradedAt time.Time) time.Time {
	if degradedAt.IsZero() {
		return feedSuccess
	}
	if feedSuccess.IsZero() || degradedAt.After(feedSuccess) {
		return time.Time{}
	}
	return feedSuccess
}

func TimestampFresh(ts, now time.Time, maxAge time.Duration) bool {
	return timestampFresh(ts, now, maxAge)
}

func timestampFresh(ts, now time.Time, maxAge time.Duration) bool {
	if ts.IsZero() {
		return false
	}
	if ts.After(now.Add(FutureTimestampTolerance)) {
		return false
	}
	return now.Sub(ts) <= maxAge
}
