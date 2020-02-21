package main

import (
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"net/http"
	"os"
	"sync"
)

var (
	metricsPath   string = "/metrics"
	listenAddress string = ":9677"
	awsRegion     string = ""
	maxResults    int64  = 10
	namespace     string = "ecs"
	debug         bool   = false
	configFile    string = ""
	log                  = logrus.New()

	up = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Was the last query of ecs successful.",
		[]string{"region", "island"},
		nil,
	)
	clusterCount = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "clusters_total"),
		"The total number of ecs clusters.",
		[]string{"region", "island"},
		nil,
	)
	serviceCount = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_total"),
		"The total number of services.",
		[]string{"region", "island", "ecsCluster"},
		nil,
	)
	serviceDesired = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "service_desired_tasks_total"),
		"The number of tasks to have running.",
		[]string{"region", "island", "ecsCluster", "service"},
		nil,
	)
	servicePending = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "service_pending_tasks_total"),
		"The number of tasks that are in the PENDING state.",
		[]string{"region", "island", "ecsCluster", "service"},
		nil,
	)
	serviceRunning = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "service_running_tasks_total"),
		"The number of tasks that are in the RUNNING state.",
		[]string{"region", "island", "ecsCluster", "service"},
		nil,
	)
	serviceDeployments = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "service_deployments_total"),
		"The number of deployments a service has.",
		[]string{"region", "island", "ecsCluster", "service"},
		nil,
	)
)

func init() {
	log.Out = os.Stdout
	log.Formatter = logrus.Formatter(&logrus.JSONFormatter{})
}

type Config struct {
	Roles map[string]string `yaml:"roles"`
}

type ECSCluster struct {
	ID   string
	Name string
}
type ECSService struct {
	ID, Name                                              string
	Deployments, DesiredTasks, RunningTasks, PendingTasks int64
}
type ecsResult struct {
	result []*ECSService
	err    error
}
type Exporter struct {
	config             Config
	region             string
	up                 *prometheus.Desc
	clusterCount       *prometheus.Desc
	serviceCount       *prometheus.Desc
	serviceDesired     *prometheus.Desc
	servicePending     *prometheus.Desc
	serviceRunning     *prometheus.Desc
	serviceDeployments *prometheus.Desc
}

func NewExporter(region string, cfg Config) (*Exporter, error) {
	return &Exporter{
		config:             cfg,
		region:             region,
		up:                 up,
		clusterCount:       clusterCount,
		serviceCount:       serviceCount,
		serviceDesired:     serviceDesired,
		servicePending:     servicePending,
		serviceRunning:     serviceRunning,
		serviceDeployments: serviceDeployments,
	}, nil
}
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	ch <- clusterCount
	ch <- serviceCount
	ch <- serviceDesired
	ch <- servicePending
	ch <- serviceRunning
	ch <- serviceDeployments
}
func (e *Exporter) worker(role string, island string, wg *sync.WaitGroup, ch chan<- prometheus.Metric) {
	defer wg.Done()
	var sWg sync.WaitGroup
	var client *ecs.ECS
	var err error
	if role == "" && island == "" {
		client, err = e.getClient(role)
	} else {
		client, err = e.getSTSClient(role)
	}
	if err != nil {
		log.Error("error aws session")
		return
	}
	clusters, err := e.getClusters(client)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(up, prometheus.GaugeValue, 0, e.region, island)
		log.Fatalf("error getting ecs clusters: %v", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(up, prometheus.GaugeValue, 1, e.region, island)
	ch <- prometheus.MustNewConstMetric(clusterCount, prometheus.GaugeValue, float64(len(clusters)), e.region, island)
	for _, cluster := range clusters {
		sWg.Add(1)
		go func(cluster ECSCluster, sWg *sync.WaitGroup) {
			defer sWg.Done()
			services, err := e.getServices(client, &cluster)
			if err != nil {
				log.Errorf("error getting services: %v", err)
				return
			}
			ch <- prometheus.MustNewConstMetric(serviceCount, prometheus.GaugeValue, float64(len(services)), e.region, island, cluster.Name)
			for _, service := range services {
				ch <- prometheus.MustNewConstMetric(serviceDesired, prometheus.GaugeValue, float64(service.DesiredTasks), e.region, island, cluster.Name, service.Name)
				ch <- prometheus.MustNewConstMetric(servicePending, prometheus.GaugeValue, float64(service.PendingTasks), e.region, island, cluster.Name, service.Name)
				ch <- prometheus.MustNewConstMetric(serviceRunning, prometheus.GaugeValue, float64(service.RunningTasks), e.region, island, cluster.Name, service.Name)
				ch <- prometheus.MustNewConstMetric(serviceDeployments, prometheus.GaugeValue, float64(service.Deployments), e.region, island, cluster.Name, service.Name)
			}
		}(*cluster, &sWg)
	}
	sWg.Wait()
}
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	islands := map[string]string{"": ""}
	if len(e.config.Roles) != 0 {
		islands = e.config.Roles
	}
	var wg sync.WaitGroup
	for k, v := range islands {
		island := k
		role := v
		wg.Add(1)
		go e.worker(role, island, &wg, ch)
	}
	wg.Wait()
}
func (e *Exporter) getSTSClient(role string) (*ecs.ECS, error) {
	ses := session.New(&aws.Config{Region: aws.String(e.region)})
	if ses == nil {
		return nil, fmt.Errorf("error creating aws session")
	}
	creds := stscreds.NewCredentials(ses, role)
	client := ecs.New(ses, &aws.Config{Credentials: creds})
	return client, nil
}

func (e *Exporter) getClient(role string) (*ecs.ECS, error) {
	ses := session.New(&aws.Config{Region: aws.String(e.region)})
	if ses == nil {
		return nil, fmt.Errorf("error creating aws session")
	}
	client := ecs.New(ses)
	return client, nil
}
func (e *Exporter) getClusters(client *ecs.ECS) ([]*ECSCluster, error) {
	clusterArns := []*string{}
	clusters := []*ECSCluster{}
	listClusterParams := &ecs.ListClustersInput{
		MaxResults: aws.Int64(maxResults),
	}
	for {
		resp, err := client.ListClusters(listClusterParams)
		if err != nil {
			return nil, err
		}

		for _, c := range resp.ClusterArns {
			clusterArns = append(clusterArns, c)
		}
		if resp.NextToken == nil || aws.StringValue(resp.NextToken) == "" {
			break
		}
		listClusterParams.NextToken = resp.NextToken
	}
	log.Debugf("got %v clusters", len(clusterArns))
	describeClusterParams := &ecs.DescribeClustersInput{
		Clusters: clusterArns,
	}
	resp, err := client.DescribeClusters(describeClusterParams)
	if err != nil {
		return nil, err
	}
	for _, c := range resp.Clusters {
		cluster := &ECSCluster{
			ID:   aws.StringValue(c.ClusterArn),
			Name: aws.StringValue(c.ClusterName),
		}
		clusters = append(clusters, cluster)
	}
	return clusters, nil
}
func (e *Exporter) getServices(client *ecs.ECS, cluster *ECSCluster) ([]*ECSService, error) {
	serviceArns := []*string{}
	counter := 0
	resC := make(chan ecsResult)
	listServicesParams := &ecs.ListServicesInput{
		Cluster:    aws.String(cluster.ID),
		MaxResults: aws.Int64(maxResults),
	}
	for {
		resp, err := client.ListServices(listServicesParams)
		if err != nil {
			return nil, err
		}

		for _, s := range resp.ServiceArns {
			serviceArns = append(serviceArns, s)
		}
		if resp.NextToken == nil || aws.StringValue(resp.NextToken) == "" {
			break
		}
		listServicesParams.NextToken = resp.NextToken
	}
	log.Debugf("got %v services on the %v cluster.", len(serviceArns), cluster.Name)
	for i := 0; i <= len(serviceArns)/int(maxResults); i++ {
		pos := i * int(maxResults)
		if pos >= len(serviceArns) {
			break
		}
		end := pos + int(maxResults)
		var servicesBlock []*string
		if end > len(serviceArns) {
			servicesBlock = serviceArns[pos:]
		} else {
			servicesBlock = serviceArns[pos:end]
		}
		counter++
		go func(block []*string) {
			services := []*ECSService{}
			describeServicesParams := &ecs.DescribeServicesInput{
				Services: block,
				Cluster:  aws.String(cluster.ID),
			}
			resp, err := client.DescribeServices(describeServicesParams)
			if err != nil {
				resC <- ecsResult{nil, err}
			}
			for _, s := range resp.Services {
				deployments := int64(len(s.Deployments))
				service := &ECSService{
					ID:           aws.StringValue(s.ServiceArn),
					Name:         aws.StringValue(s.ServiceName),
					DesiredTasks: aws.Int64Value(s.DesiredCount),
					RunningTasks: aws.Int64Value(s.RunningCount),
					PendingTasks: aws.Int64Value(s.PendingCount),
					Deployments:  deployments,
				}
				services = append(services, service)
			}
			resC <- ecsResult{services, nil}
		}(servicesBlock)
	}
	services := []*ECSService{}
	for i := 0; i < counter; i++ {
		res := <-resC
		if res.err != nil {
			return services, res.err
		}
		services = append(services, res.result...)
	}
	return services, nil
}

func main() {
	var cfg Config
	flag.StringVar(&metricsPath, "web.telemetry-path", metricsPath, "The path where metrics will be exposed")
	flag.StringVar(&listenAddress, "web.listen-address", listenAddress, "Address to listen on")
	flag.StringVar(&awsRegion, "aws.region", awsRegion, "The AWS region to get metrics from")
	flag.BoolVar(&debug, "debug", debug, "Run exporter in debug mode")
	flag.StringVar(&configFile, "config", configFile, "Config file path")
	flag.Parse()
	if debug {
		log.SetLevel(logrus.DebugLevel)
	}
	if configFile != "" {
		f, err := os.Open(configFile)
		if err != nil {
			log.Fatalf("Error getting config file: %v", err)
		}
		decoder := yaml.NewDecoder(f)
		err = decoder.Decode(&cfg)
		if err != nil {
			log.Fatalf("Error parsing config: %v", err)
		}
	}
	if awsRegion == "" {
		log.Fatalf("Please supply an AWS region")
	}
	exporter, err := NewExporter(awsRegion, cfg)
	if err != nil {
		log.Fatal("bad time getting exporter")
	}
	prometheus.MustRegister(exporter)

	http.Handle(metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>ECS Exporter</title></head>
             <body>
             <h1>ECS Exporter</h1>
             <p><a href='` + metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})
	log.Infof("Listening on %v", listenAddress)
	log.Fatal(http.ListenAndServe(listenAddress, nil))
}
