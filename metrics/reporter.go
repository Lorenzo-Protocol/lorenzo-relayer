package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type ReporterMetrics struct {
	Registry                    *prometheus.Registry
	SuccessfulHeadersCounter    prometheus.Counter
	FailedHeadersCounter        prometheus.Counter
	SecondsSinceLastHeaderGauge prometheus.Gauge
	NewReportedHeaderGaugeVec   *prometheus.GaugeVec
}

func NewReporterMetrics() *ReporterMetrics {
	registry := prometheus.NewRegistry()
	registerer := promauto.With(registry)

	metrics := &ReporterMetrics{
		Registry: registry,
		SuccessfulHeadersCounter: registerer.NewCounter(prometheus.CounterOpts{
			Name: "lrzrelayer_reporter_reported_headers",
			Help: "The total number of BTC headers reported to Lorenzo",
		}),
		FailedHeadersCounter: registerer.NewCounter(prometheus.CounterOpts{
			Name: "lrzrelayer_reporter_failed_headers",
			Help: "The total number of failed BTC headers to Lorenzo",
		}),
		SecondsSinceLastHeaderGauge: registerer.NewGauge(prometheus.GaugeOpts{
			Name: "lrzrelayer_reporter_since_last_header_seconds",
			Help: "Seconds since the last successful reported BTC header to Lorenzo",
		}),
		NewReportedHeaderGaugeVec: registerer.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "lrzrelayer_reporter_new_btc_header",
				Help: "The metric of a new BTC header reported to Lorenzo",
			},
			[]string{
				// the id of the reported BTC header
				"id",
			},
		),
	}
	return metrics
}

func (sm *ReporterMetrics) RecordMetrics() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for range ticker.C {
			// will be reset when a header is successfully sent
			sm.SecondsSinceLastHeaderGauge.Inc()
		}
	}()

}
