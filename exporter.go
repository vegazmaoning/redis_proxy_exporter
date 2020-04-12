package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/gomodule/redigo/redis"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	constMetricGauges = map[string][]string{
		"UsedMemory":           []string{"used_memory", "Current alloc memory"},
		"MaxMemory":            []string{"max_memory", "Current max memory setting"},
		"MaxRSS":               []string{"max_rss", "Max rss used"},
		"UsedCpuSys":           []string{"used_cpu_sys", "Used cpu sys"},
		"UsedCpuUser":          []string{"used_cpu_user", "Used cpu user"},
		"UsedCpu":              []string{"used_cpu", "Used cpu"},
		"Accept":               []string{"accept", "Acceptted connections"},
		"ClientConnections":    []string{"client_connections", "Current client connections"},
		"TotalRequests":        []string{"total_requests", "Total requests"},
		"TotalResponses":       []string{"total_respones", "Total responses"},
		"TotalRecvClientBytes": []string{"total_recv_client_bytes", "Total recv client bytes"},
		"TotalSendServerBytes": []string{"total_send_server_bytes", "Total send server bytes"},
		"TotalRecvServerBytes": []string{"total_recv_server_bytes", "Total recv server bytes"},
		"TotalSendClientBytes": []string{"total_send_client_bytes", "Total send client bytes"},
	}
	serverMetricGauges = map[string][]string{
		"Connections": []string{"connections", "Server connections"},
		"Connect":     []string{"connect", "Server connect count"},
		"Requests":    []string{"requests", "Server requests"},
		"Responses":   []string{"responses", "Server responses"},
		"SendBytes":   []string{"send_bytes", "Server send bytes"},
		"RecvBytes":   []string{"recv_bytes", "Server recv bytes"},
	}
	serverLabels = []string{"server"}
)

const (
	constMetric = iota
	serverMetric
	latencyMetric
)

type Exporter struct {
	sync.Mutex
	proxyAddr string
	password  string
	namespace string
	// constMetricGauges map[string]prometheus.Gauge
	constMetricGauges  map[string][]string
	serverMetricGauges map[string]*prometheus.GaugeVec
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.Lock()
	defer e.Unlock()
	e.scrape(ch)
	for _, m := range e.serverMetricGauges {
		m.Collect(ch)
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	// for _, v := range e.constMetricGauges {
	// 	ch <- v.Desc()
	// }
	for _, v := range e.constMetricGauges {
		ch <- newMetricDescr(e.namespace, v[0], v[1], nil)
	}
	for _, v := range e.serverMetricGauges {
		v.Describe(ch)
	}
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) error {
	client, err := redis.Dial("tcp", e.proxyAddr)
	fmt.Println(client)
	//defer client.Close()
	if err != nil {
		fmt.Println(err)
		return err
	}
	if e.password != "" {
		if _, err := client.Do("AUTH", e.password); err != nil {
			fmt.Println(err)
			return err
		}
	}
	return e.parseInfo(ch, client)
}

func (e *Exporter) parseInfo(ch chan<- prometheus.Metric, client redis.Conn) error {
	info, err := redis.String(client.Do("INFO"))
	if err != nil {
		return err
	}
	lines := strings.Split(info, "\n")
	var infoType = 0
	server := ""
	servers := []string{}
	for _, line := range lines {
		switch infoType {
		case constMetric:
			if strings.HasPrefix(line, "# Servers") {
				infoType = serverMetric
				continue
			} else if strings.HasPrefix(line, "# LatencyMonitor") {
				infoType = latencyMetric
				continue
			}
			str := strings.Split(line, ":")
			_, ok := constMetricGauges[str[0]]
			if ok {
				value, err := strconv.ParseFloat(str[1], 64)
				fmt.Println(value)
				if err == nil {
					e.registerConstMetric(ch, constMetricGauges[str[0]][0], value, constMetricGauges[str[0]][1], prometheus.GaugeValue)
				}
			}
		case serverMetric:
			str := strings.Split(line, ":")
			if len(str) >= 2 {
				g, ok := e.serverMetricGauges[str[0]]
				if ok {
					v, err := strconv.ParseFloat(str[1], 64)
					if err == nil {
						g.WithLabelValues(server).Set(v)
					}
				} else if str[0] == "Server" {
					server = str[1] + ":" + str[2]
					servers = append(servers, server)
				}
			} else if strings.HasPrefix(line, "# LatencyMonitor") {
				infoType = latencyMetric
			}
		}
	}
	return nil
}

func (e *Exporter) registerConstMetric(ch chan<- prometheus.Metric, metric string, val float64, docString string, valType prometheus.ValueType, labelValues ...string) {
	descr := newMetricDescr(e.namespace, metric, docString, nil)
	if m, err := prometheus.NewConstMetric(descr, valType, val, labelValues...); err == nil {
		ch <- m
	} else {

		//log.Debugf("NewConstMetric() err: %s", err)
	}
}

func newMetricDescr(namespace string, metricName string, docString string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc(prometheus.BuildFQName(namespace, "", metricName), docString, labels, nil)
}

func NewExporter(proxyAddr, password, namespace string) *Exporter {
	e := &Exporter{
		proxyAddr:          proxyAddr,
		password:           password,
		namespace:          namespace,
		constMetricGauges:  constMetricGauges,
		serverMetricGauges: map[string]*prometheus.GaugeVec{},
	}
	// for key, metric := range constMetricGauges {
	// 	e.constMetricGauges[key] = prometheus.NewGauge(
	// 		prometheus.GaugeOpts{
	// 		Namespace:   e.namespace,
	// 		Name:        metric[0],
	// 		Help:        metric[1],
	// 		}
	// 	)
	// }
	for key, metric := range serverMetricGauges {
		e.serverMetricGauges[key] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      metric[0],
				Help:      metric[1],
			}, serverLabels)
	}
	return e
}
