package main

import (
	"flag"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

func getEnv(key string, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultValue
}

func main() {
	var (
		proxyAddr     = flag.String("proxy.addr", getEnv("PROXY_ADDR", ":7617"), "Redis proxy address(ip:port)")
		proxyPassword = flag.String("proxy.password", getEnv("PROXY_PASSWORD", ""), "Proxy password")
		namespace     = flag.String("namespace", "redis_proxy", "Namespace for the metric")
		timeout       = flag.String("timeout", "15s", "Timeout for connect to redis proxy")
		listenAddr    = flag.String("web.listen", ":9122", "Listen address")
		logLevel      = flag.String("loglevel", "INFO", "loglevel(DEBUG or INFO)")
	)
	flag.Parse()
	connectTimeout, err := time.ParseDuration(*timeout)
	if err != nil {

	}
	switch *logLevel {
	case "DEBUG":
		log.SetLevel(log.DebugLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
	exporter := NewExporter(*proxyAddr, *proxyPassword, *namespace, connectTimeout)
	prometheus.MustRegister(exporter)
	http.Handle("/metrics", promhttp.Handler())
	log.Infof("Providing metrics at %s/metrics", *listenAddr)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}
