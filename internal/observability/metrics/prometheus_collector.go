package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// PrometheusCollector implementa domain.MetricsCollector usando Prometheus
type PrometheusCollector struct {
	transactionCounter *prometheus.CounterVec
	transactionLatency prometheus.Histogram
	businessMetrics    *prometheus.GaugeVec
	errorCounter       *prometheus.CounterVec
}

func NewPrometheusCollector() *PrometheusCollector {
	return &PrometheusCollector{
		// Contador de transações por status
		transactionCounter: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "transactions_total",
				Help: "Total number of processed transactions",
			},
			[]string{"status"},
		),

		// Histograma de latência das transações
		transactionLatency: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "transaction_duration_seconds",
				Help:    "Transaction processing duration in seconds",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~32s
			},
		),

		// Métricas de negócio (valores, limites, etc.)
		businessMetrics: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "business_metrics",
				Help: "Business-specific metrics",
			},
			[]string{"metric_name", "status", "cliente_id"},
		),

		// Contador de erros por tipo
		errorCounter: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "errors_total",
				Help: "Total number of errors by type",
			},
			[]string{"error_type"},
		),
	}
}

// IncrementTransactionCounter incrementa contador de transações
func (c *PrometheusCollector) IncrementTransactionCounter(status string) {
	c.transactionCounter.WithLabelValues(status).Inc()
}

// RecordTransactionLatency registra latência de transação
func (c *PrometheusCollector) RecordTransactionLatency(duration float64) {
	c.transactionLatency.Observe(duration)
}

// RecordBusinessMetric registra métricas de negócio
func (c *PrometheusCollector) RecordBusinessMetric(metricName string, value float64, labels map[string]string) {
	// Extrai labels específicos
	status := labels["status"]
	clienteID := labels["cliente_id"]

	c.businessMetrics.WithLabelValues(metricName, status, clienteID).Set(value)
}

// IncrementErrorCounter incrementa contador de erros
func (c *PrometheusCollector) IncrementErrorCounter(errorType string) {
	c.errorCounter.WithLabelValues(errorType).Inc()
}

// GetRegistry retorna o registry padrão do Prometheus
func (c *PrometheusCollector) GetRegistry() *prometheus.Registry {
	return prometheus.DefaultRegisterer.(*prometheus.Registry)
}
