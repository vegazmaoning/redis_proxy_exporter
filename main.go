package main

import (
	"os"
	"net/http"
	"time"
	"log"
	"fmt"

	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Options struct {
	ProxyAddr string `short:"h" long:"proxy.addr" value-name:"proxy_address" default:"localhost:7617" description:"Redis proxy address(ip:port)"`
	Password string `short:"p" long:"proxy.password" value-name:"proxy_password" default:"" description:"Redis proxy password"`
	Namespace string `short:"n" long:"namespace" value-name:"namespace" default:"redis_proxy" description:"Namespace for the metric"`
	Timeout  time.Duration `short:"t" long:"timeout" value-name:"timeout" default:"15s" description:"Timeout for connect to redis proxy"`
//	ProxyMetricOnly bool `long:"proxy_metric_only" value-name:"proxy_metric_only" default:"false" description:"Whether to also export go runtime metrics"`
	ListenAddr string `long:"web.listen" value-name:"web listen" default:":9122" description:"Listen address"`
}

var opt Options
func main() {
	_, err := flags.ParseArgs(&opt, os.Args[1:])
	if err != nil {
		fmt.Println("parse args error %v", err)
	}
	exporter := NewExporter(opt.ProxyAddr, opt.Password, opt.Namespace)
	prometheus.MustRegister(exporter)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(opt.ListenAddr, nil))
}
