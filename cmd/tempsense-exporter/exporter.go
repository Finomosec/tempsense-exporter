package main

import (
	"flag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
)

var (
	flagAddr       = flag.String("address", ":9181", "The address to listen on for HTTP requests.")
	flagSensorsCsv = flag.String("sensors-csv", "", "Path to the sensors.csv with display names. (default \"<binary dir>/../cfg/sensors.csv\")")
)

func main() {
	flag.Parse()

	collector := NewTempsenseCollector()
	collector.csvPath = *flagSensorsCsv
	prometheus.MustRegister(collector)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*flagAddr, nil))
}
