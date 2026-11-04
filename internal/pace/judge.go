package pace

import "time"

// verdict is what the recent response times say about the server.
type verdict int

const (
	noVerdict verdict = iota // too few samples, or in between
	slow                     // answering well above its baseline, or over the ceiling
	healthy                  // answering close to its baseline
)

// judgeParams are the latency thresholds; see the package comment.
type judgeParams struct {
	SlowFactor    float64
	SlowMargin    time.Duration
	SlowCeiling   time.Duration
	HealthyFactor float64
}

// latencyJudge is the median-of-recent-responses test shared by the listing
// Pacer and the download Gate. Not safe for concurrent use: its owner's mutex
// guards it.
type latencyJudge struct {
	window   []time.Duration // recent successful latencies (ring, unordered)
	next     int
	baseline time.Duration // 0 until minSamples have been seen
}

// add records one successful response and judges the window. m is the
// window's median (0 while there are too few samples).
func (j *latencyJudge) add(lat time.Duration, jp judgeParams) (v verdict, m time.Duration) {
	if len(j.window) < latencyWindow {
		j.window = append(j.window, lat)
	} else {
		j.window[j.next] = lat
		j.next = (j.next + 1) % latencyWindow
	}
	if len(j.window) < minSamples {
		return noVerdict, 0
	}
	m = median(j.window)
	switch {
	case j.baseline == 0 || m < j.baseline:
		j.baseline = m
	default:
		j.baseline += time.Duration(float64(m-j.baseline) * baselineCreep)
	}
	slowAt := time.Duration(float64(j.baseline) * jp.SlowFactor)
	if alt := j.baseline + jp.SlowMargin; alt > slowAt {
		slowAt = alt
	}
	if slowAt > jp.SlowCeiling {
		slowAt = jp.SlowCeiling
	}
	switch {
	case m > slowAt:
		return slow, m
	case (m <= time.Duration(float64(j.baseline)*jp.HealthyFactor) || m <= j.baseline+jp.SlowMargin/2) &&
		m <= jp.SlowCeiling:
		return healthy, m
	}
	return noVerdict, m
}

// reset empties the window, so the next judgement is on responses at the new
// pace only. The baseline is kept.
func (j *latencyJudge) reset() {
	j.window = j.window[:0]
	j.next = 0
}
