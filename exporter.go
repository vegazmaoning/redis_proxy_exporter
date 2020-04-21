package main

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
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
	serverLabels      = []string{"server"}
	latencyLabels     = []string{"latency"}
	servLatencyLabels = []string{"server", "latency"}
	latencyBuckets    = prometheus.LinearBuckets(100, 100, 100)
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
	timeout   time.Duration
	// constMetricGauges map[string]prometheus.Gauge
	constMetricGauges  map[string][]string
	serverMetricGauges map[string]*prometheus.GaugeVec
	latencyDesc        *prometheus.Desc
	servLatencyDesc    *prometheus.Desc
	latencyMetrics     []prometheus.Metric
	metricDescriptions map[string]*prometheus.Desc
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	log.Debugf("start scrape...")
	e.Lock()
	defer e.Unlock()
	e.scrape(ch)
	for _, m := range e.serverMetricGauges {
		m.Collect(ch)
	}
	for _, m := range e.latencyMetrics {
		ch <- m
	}
	e.latencyMetrics = e.latencyMetrics[:0]
	log.Debugf("scrape complete.")
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
	for _, desc := range e.metricDescriptions {
		ch <- desc
	}
	ch <- e.latencyDesc
	ch <- e.servLatencyDesc
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) {
	options := []redis.DialOption{
		redis.DialConnectTimeout(e.timeout),
		redis.DialReadTimeout(e.timeout),
		redis.DialWriteTimeout(e.timeout),
	}
	client, err := redis.Dial("tcp", e.proxyAddr, options...)
	if err != nil {
		log.Infof("connect to proxy error err:%s", err)
		e.registerConstMetric(ch, "up", 0, "Redis Proxy instance health", prometheus.GaugeValue)
		return
	} else {
		e.registerConstMetric(ch, "up", 1, "Redis Proxy instance health", prometheus.GaugeValue)
	}
	defer client.Close()
	if e.password != "" {
		if _, err := client.Do("AUTH", e.password); err != nil {
			log.Errorf("authority failed  err:%s", err)
			return
		}
	}
	e.parseInfo(ch, client)
}

func (e *Exporter) parseInfo(ch chan<- prometheus.Metric, client redis.Conn)  {
	info, err := redis.String(client.Do("INFO"))
	if err != nil {
		log.Errorf("execute info error err:%s", err)
		return
	}
	lines := strings.Split(info, "\n")
	var infoType = 0
	server := ""
	latency := ""
	servers := []string{}
	for len(lines) > 0 {
		line := lines[0]
		lines = lines[1:]
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
		case latencyMetric:
			if strings.HasPrefix(line, "LatencyMonitorName") {
				latency = strings.Split(line, ":")[1]
				buckets, last, lineNum, ok := e.parseBuckets(lines)
				if ok {
					e.addLatencyMetrics(buckets, last, false, latency)
				}
				lines = lines[lineNum:]
			}
		}
	}
	for _, server := range servers {
		r, err := redis.String(client.Do("INFO", "ServerLatency", server))
		if err != nil {
			log.Errorf("redis %s INFO ServerLatency %s error:%q", e.proxyAddr, server, err)
			continue
		}
		latency = ""
		lines = strings.Split(r, "\n")
		for len(lines) > 0 {
			line := lines[0]
			lines = lines[1:]
			if strings.HasPrefix(line, "ServerLatencyMonitorName") {
				latency = strings.Split(line, " ")[1]
				buckets, last, lineNum, ok := e.parseBuckets(lines)
				if ok {
					e.addLatencyMetrics(buckets, last, true, server, latency)
				}
				lines = lines[lineNum:]
			}
		}
	}
}

func (e *Exporter) registerConstMetric(ch chan<- prometheus.Metric, metric string, val float64, docString string, valType prometheus.ValueType, labelValues ...string) {
	descr := e.metricDescriptions[metric]
	if descr == nil {

		descr = newMetricDescr(e.namespace, metric, docString, nil)
	}
	if m, err := prometheus.NewConstMetric(descr, valType, val, labelValues...); err == nil {
		ch <- m
	} else {

		log.Errorf("NewConstMetric() err: %s", err)
	}
}

func (e *Exporter) parseBuckets(lines []string) (buckets []Bucket, last Bucket, lineNum int, ok bool) {
	ok = false
	buckets = make([]Bucket, 0)
	for lineNum = 0; lineNum < len(lines); {
		line := lines[lineNum]
		lineNum++
		s := strings.Fields(line)
		if len(s) < 4 {
			return
		}
		bound, err := strconv.ParseUint(s[1], 10, 64)
		if err != nil {
			return
		}
		elapsed, err := strconv.ParseUint(s[2], 10, 64)
		if err != nil {
			return
		}
		count, err := strconv.ParseUint(s[3], 10, 64)
		if err != nil {
			return
		}
		if s[0] == "<=" {
			buckets = append(buckets, Bucket{bound, elapsed, count})
		} else if s[0] == ">" {
			last.bound, last.value, last.count = bound, elapsed, count
		} else if s[0] == "T" {
			ok = true
			return
		}
	}
	return
}

func (e *Exporter) addLatencyMetrics(buckets []Bucket, last Bucket, server bool, labels ...string) {
	vc := make(map[float64]uint64)
	j := 0
	value := float64(0)
	count := uint64(0)
	for i, b := range latencyBuckets {
		for j < len(buckets) {
			if float64(buckets[j].bound) <= b {
				value += float64(buckets[j].value)
				count += buckets[j].count
				j++
			} else {
				break
			}
		}
		vc[latencyBuckets[i]] = count
	}
	log.Infof("bucket %v", buckets)
	for ; j < len(buckets); j++ {
		value += float64(buckets[j].value)
		count += buckets[j].count
	}
	count += last.count
	var h prometheus.Metric
	if server {
		h = prometheus.MustNewConstHistogram(e.servLatencyDesc, count, value, vc, labels...)
	} else {
		h = prometheus.MustNewConstHistogram(e.latencyDesc, count, value, vc, labels...)
	}
	e.latencyMetrics = append(e.latencyMetrics, h)
}

type Bucket struct {
	bound uint64
	value uint64
	count uint64
}

func newMetricDescr(namespace string, metricName string, docString string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc(prometheus.BuildFQName(namespace, "", metricName), docString, labels, nil)
}

func NewExporter(proxyAddr, password, namespace string, timeout time.Duration) *Exporter {
	e := &Exporter{
		proxyAddr:          proxyAddr,
		password:           password,
		namespace:          namespace,
		timeout:            timeout,
		constMetricGauges:  constMetricGauges,
		serverMetricGauges: map[string]*prometheus.GaugeVec{},
		metricDescriptions: map[string]*prometheus.Desc{},
		latencyDesc: prometheus.NewDesc(
			namespace+"_latency",
			"Latency",
			latencyLabels,
			nil),
		servLatencyDesc: prometheus.NewDesc(
			namespace+"_server_latency",
			"Server latency",
			servLatencyLabels,
			nil),
		latencyMetrics: []prometheus.Metric{},
	}
	for key, metric := range serverMetricGauges {
		e.serverMetricGauges[key] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: e.namespace,
				Name:      metric[0],
				Help:      metric[1],
			}, serverLabels)
	}
	for metric, help := range map[string]string{
		"up": "Redis Proxy instance health",
	} {
		e.metricDescriptions[metric] = newMetricDescr(e.namespace, metric, help, nil)
	}
	return e
}

