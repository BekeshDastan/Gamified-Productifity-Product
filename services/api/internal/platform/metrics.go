package platform

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var requestMetrics = struct {
	sync.Mutex
	values map[string]*requestMetric
}{values: make(map[string]*requestMetric)}

type requestMetric struct {
	count   uint64
	seconds float64
	buckets [8]uint64
}

var bounds = [8]float64{.01, .025, .05, .1, .25, .5, 1, 5}

func observeRequest(route, method string, status int, elapsed time.Duration) {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		method = "OTHER"
	}
	if status == 0 {
		status = http.StatusOK
	}
	label := fmt.Sprintf("route=%q,method=%q,status_class=%q", route, method, fmt.Sprintf("%dxx", status/100))
	requestMetrics.Lock()
	defer requestMetrics.Unlock()
	value := requestMetrics.values[label]
	if value == nil {
		value = &requestMetric{}
		requestMetrics.values[label] = value
	}
	value.count++
	value.seconds += elapsed.Seconds()
	for i, bound := range bounds {
		if elapsed.Seconds() <= bound {
			value.buckets[i]++
		}
	}
}

// Labels contain registered route patterns only, never account IDs or URLs.
func HTTPMetrics() string {
	requestMetrics.Lock()
	defer requestMetrics.Unlock()
	var out strings.Builder
	out.WriteString("# TYPE ascend_http_duration_seconds histogram\n")
	keys := make([]string, 0, len(requestMetrics.values))
	for key := range requestMetrics.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		v := requestMetrics.values[key]
		fmt.Fprintf(&out, "ascend_http_duration_seconds_count{%s} %d\nascend_http_duration_seconds_sum{%s} %g\n", key, v.count, key, v.seconds)
		for i, bound := range bounds {
			fmt.Fprintf(&out, "ascend_http_duration_seconds_bucket{%s,le=%q} %d\n", key, fmt.Sprint(bound), v.buckets[i])
		}
		fmt.Fprintf(&out, "ascend_http_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", key, v.count)
	}
	return out.String()
}
