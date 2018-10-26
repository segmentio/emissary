package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/apex/log"
	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/pkg/errors"
	"github.com/segmentio/conf"
	"github.com/segmentio/consul-go"
	"github.com/segmentio/emissary"
	"github.com/segmentio/events"
	"github.com/segmentio/stats"
	"github.com/segmentio/stats/datadog"
	"github.com/segmentio/stats/procstats"
	"google.golang.org/grpc"
)

type emissaryConfig struct {
	Port      int             `conf:"port"`
	AdminPort int             `conf:"adminport"`
	Consul    string          `conf:"consul"`
	Dogstatsd dogstatsdConfig `conf:"dogstatsd" help:"dogstatsd Configuration"`
}

type dogstatsdConfig struct {
	Address    string        `conf:"address" help:"Address of the dogstatsd agent that will receive metrics"`
	BufferSize int           `conf:"buffer-size" help:"Size of the statsd metrics buffer" validate:"min=0"`
	FlushEvery time.Duration `conf:"flush-every" help:"Flush AT LEAST this frequently"`
}

// version will be set by CI using ld_flags to the git SHA on which the binary was built
var version = "unknown"

func eds(ctx context.Context, cancel context.CancelFunc, client *consul.Client, config *emissaryConfig) {
	defer configureDogstatsd(ctx, config.Dogstatsd, "eds")()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		log.WithError(errors.Wrap(err, "starting EndpointDiscoveryService"))
		return
	}

	mux := http.DefaultServeMux
	mux.HandleFunc("/internal/health", func(w http.ResponseWriter, r *http.Request) {
		var recv string
		err := client.Get(context.Background(), "/v1/status/leader", nil, &recv)
		if err != nil {
			w.WriteHeader(500)
			return
		}
	})

	go func() {
		if err := http.ListenAndServe(fmt.Sprintf(":%d", config.AdminPort), nil); err != nil {
			panic(fmt.Sprintf("unable to bind to admin port %d, exiting.", config.AdminPort))
		}
	}()

	grpcServer := grpc.NewServer()
	xds.RegisterEndpointDiscoveryServiceServer(grpcServer, emissary.NewEdsService(ctx, client))
	log.Infof("starting emissary EndpointDiscoveryService service on port: %d", config.Port)
	go func() {
		err := grpcServer.Serve(lis)
		log.Errorf("error in eds grpc server", err)
		cancel()
	}()
}

func main() {
	ld := conf.Loader{
		Name: "emissary",
		Args: os.Args[1:],
		Commands: []conf.Command{
			{Name: "eds", Help: "Run the EndpointDiscoveryService"},
			// Right now we just support eds but there are several other xDS service endpoints we can implement
			// lds, rds, cds ads etc.
			//{Name: "cds", Help: "Run the ClusterDiscoveryService"},
			//{Name: "rds", Help: "Run the RouteDiscoveryService"},
			//{Name: "lds", Help: "Run the ListenerDiscoveryService"},
		},
	}

	ctx, cancel := events.WithSignals(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	events.DefaultLogger.EnableDebug = false
	config := &emissaryConfig{
		Port:      9001,
		AdminPort: 9002,
		Consul:    "localhost:8500",
		Dogstatsd: defaultDogstatsdConfig(),
	}

	switch cmd, _ := conf.LoadWith(config, ld); cmd {
	case "eds":
		client := &consul.Client{Address: config.Consul}
		eds(ctx, cancel, client, config)
	default:
		panic("unknown command: " + cmd)
	}

	<-ctx.Done()
	log.Info("exiting emissary")
}

func defaultDogstatsdConfig() dogstatsdConfig {
	return dogstatsdConfig{
		BufferSize: 1024,
		FlushEvery: 5 * time.Second,
	}
}

func configureDogstatsd(ctx context.Context, config dogstatsdConfig, statsPrefix string) (teardown func()) {
	if config.Address != "" {
		if statsPrefix == "" {
			panic("configureDogstatsd: Invalid statsPrefix passed. Stop.")
		}

		dd := datadog.NewClientWith(datadog.ClientConfig{
			Address:    config.Address,
			BufferSize: config.BufferSize,
		})
		stats.Register(dd)
		stats.DefaultEngine.Prefix = fmt.Sprintf("emissary.%s", statsPrefix)
		stats.DefaultEngine.Tags = append(stats.DefaultEngine.Tags, stats.Tag{Name: "version", Value: version})
		stats.DefaultEngine.Tags = stats.SortTags(stats.DefaultEngine.Tags) // tags must be sorted

		c := procstats.StartCollector(procstats.NewGoMetrics())

		events.Log("Setup dogstatsd with addr:%{addr}s, buffersize:%{buffersize}d, prefix:%{pfx}s, version:%{version}s",
			config.Address, config.BufferSize, statsPrefix, version)

		go func() {
			ticker := time.NewTicker(config.FlushEvery)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					stats.Flush()
				case <-ctx.Done():
					return
				}
			}
		}()
		return func() {
			c.Close()
			stats.Flush()
		}
	}
	// nothing to be done for teardown here
	return func() {}
}
