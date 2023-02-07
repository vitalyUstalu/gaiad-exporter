package main

import (
 "github.com/joho/godotenv"
 "github.com/prometheus/client_golang/prometheus"
 "github.com/prometheus/client_golang/prometheus/promhttp"
 "net/http"
 "log"
 "encoding/json"
 "strconv"
 "time"
 "flag"
 "os"
 "crypto/tls"
 "io/ioutil"
)

const namespace = "gaiad"
const statusApi = "/status"
const networkApi = "/net_info"

var (
	tr = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client = &http.Client{Transport: tr}

	listenAddress = flag.String("listen-address", ":9101", "Address to listen")
	metricsPath = flag.String("metrics-path", "/metrics", "Path to expose metrics")
	configPath = flag.String("config-file-path", "", "Path to environment file")

	latestBlockHeight = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "latest_block_height"),
		"Current block number.",
		[]string{"node_id", "chain_id"}, nil,
	)
	latestBlockTimeDiff = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "latest_block_time_diff"),
		"Difference between current time and the latest block time.",
		[]string{"node_id", "chain_id"}, nil,
	)
	peersNumber = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "number_of_peers"),
		"Number of peers.",
		[]string{"node_id", "chain_id"}, nil,
	)
)

type Exporter struct {
	gaiadEndpoint string
}
   
func NewExporter(gaiadEndpoint string) *Exporter {
	return &Exporter{
		gaiadEndpoint: gaiadEndpoint,
	}
}
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- latestBlockHeight
	ch <- latestBlockTimeDiff
	ch <- peersNumber
}
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.updateGaiadMetricsFromApi(ch)
}

func (e *Exporter) updateGaiadMetricsFromApi(ch chan<- prometheus.Metric) {
	req, err := http.NewRequest("GET", e.gaiadEndpoint+statusApi, nil)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
	var result map[string]any
	json.Unmarshal([]byte(body), &result)
	var nodeId = result["result"].(map[string]any)["node_info"].(map[string]any)["id"].(string)
	var chainId = result["result"].(map[string]any)["node_info"].(map[string]any)["network"].(string)
	var blockHeight float64
	blockHeight, err = strconv.ParseFloat(result["result"].(map[string]any)["sync_info"].(map[string]any)["latest_block_height"].(string), 64)
	if err != nil {
		log.Fatal(err)
	}
	var blockTime time.Time
	blockTime, err = time.Parse(time.RFC3339, result["result"].(map[string]any)["sync_info"].(map[string]any)["latest_block_time"].(string))
	if err != nil {
		log.Fatal(err)
	}
	var curTime = time.Now()
	var blockTimeDiff = curTime.Sub(blockTime).Seconds()
	ch <- prometheus.MustNewConstMetric(latestBlockHeight, prometheus.CounterValue, blockHeight, nodeId, chainId)
	ch <- prometheus.MustNewConstMetric(latestBlockTimeDiff, prometheus.GaugeValue, blockTimeDiff, nodeId, chainId)
	e.updateNetworkGaiadMetricsFromApi(nodeId, chainId, ch)
}

func (e *Exporter) updateNetworkGaiadMetricsFromApi(nodeId string, chainId string, ch chan<- prometheus.Metric) {
	req, err := http.NewRequest("GET", e.gaiadEndpoint+networkApi, nil)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
	var result map[string]any
	json.Unmarshal([]byte(body), &result)
	var numPeers float64
	numPeers, err = strconv.ParseFloat(result["result"].(map[string]any)["n_peers"].(string), 64)
	if err != nil {
		log.Fatal(err)
	}
	ch <- prometheus.MustNewConstMetric(peersNumber, prometheus.GaugeValue, numPeers, nodeId, chainId)
}

func main() {
	flag.Parse()

	configFile := *configPath
	if configFile != "" {
		log.Printf("Loading %s env file.\n", configFile)
	    err := godotenv.Load(configFile)
		if err != nil {
			log.Printf("Error loading %s env file.\n", configFile)
		}
	} else {
		err := godotenv.Load()
		if err != nil {
			log.Println("Error loading .env file, assume env variables are set.")
		}
	}

	gaiadEndpoint := os.Getenv("GAIAD_ENDPOINT")

	exporter := NewExporter(gaiadEndpoint)
	prometheus.MustRegister(exporter)
    log.Printf("Using connection endpoint: %s", gaiadEndpoint)
	http.Handle(*metricsPath, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}