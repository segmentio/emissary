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
	"google.golang.org/grpc/keepalive"
)

type emissaryConfig struct {
	Port         int                  `conf:"port"          help:"Bind port."`
	AdminPort    int                  `conf:"adminport"     help:"Admin port for healthcheck."`
	Consul       string               `conf:"consul"        help:"If configure, emissary will use Consul as resolver.`
	Docker       dockerResolverConfig `conf:"docker"        help:"If configure, emissary will use Docker as resolver."`
	Dogstatsd    dogstatsdConfig      `conf:"dogstatsd"     help:"dogstatsd Configuration."`
	PollInterval time.Duration        `conf:"poll-interval" help:"Interval for the poller."`
}

type dogstatsdConfig struct {
	Address    string        `conf:"address"     help:"Address of the dogstatsd agent that will receive metrics"`
	BufferSize int           `conf:"buffer-size" help:"Size of the statsd metrics buffer" validate:"min=0"`
	FlushEvery time.Duration `conf:"flush-every" help:"Flush AT LEAST this frequently"`
}

type dockerResolverConfig struct {
	Host         string `conf:"host"          help:"Address of the Docker resolver."`
	ServicePort  int    `conf:"service-port"  help:"Private port exported by the containers."`
	ServiceLabel string `conf:"service-label" help:"Name of the label to use for service lookup."`
}

// version will be set by CI using ld_flags to the git SHA on which the binary was built
var version = "unknown"

func eds(ctx context.Context, cancel context.CancelFunc, config *emissaryConfig) *grpc.Server {
	defer configureDogstatsd(ctx, config.Dogstatsd, "eds")()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		log.WithError(errors.Wrap(err, "starting EndpointDiscoveryService"))
		return nil
	}

	mux := http.DefaultServeMux

	grpcServer := grpc.NewServer(grpc.KeepaliveParams(keepalive.ServerParameters{
		Time: 60 * time.Second,
	}))

	var eds *emissary.EdsService
	if len(config.Consul) != 0 {
		log.Infof("configuring Consul resolver: %s", config.Consul)

		client := consul.Client{Address: config.Consul}
		resolver := &consul.Resolver{
			Client: &client,
		}
		eds = emissary.NewEdsService(ctx, emissary.WithConsul(resolver, config.PollInterval))

		mux.HandleFunc("/internal/health", func(w http.ResponseWriter, r *http.Request) {
			var recv string
			err := client.Get(context.Background(), "/v1/status/leader", nil, &recv)
			if err != nil {
				w.WriteHeader(500)
				return
			}
		})

	} else if len(config.Docker.Host) != 0 {
		log.Infof("configuring Docker resolver: %s", config.Docker.Host)

		resolver := emissary.DockerResolver{
			Client: emissary.DockerClient{
				Host: config.Docker.Host,
			},
		}

		if config.Docker.ServicePort != 0 {
			emissary.DockerServicePort = config.Docker.ServicePort
		}

		if len(config.Docker.ServiceLabel) != 0 {
			emissary.DockerServiceLabel = config.Docker.ServiceLabel
		}

		eds = emissary.NewEdsService(ctx, emissary.WithDocker(&resolver, config.PollInterval))

		mux.HandleFunc("/internal/health", func(w http.ResponseWriter, r *http.Request) {
			//TODO: implement healthcheck for Docker API
			w.WriteHeader(200)
		})
	} else {
		panic("no resolver has been configured.")
	}

	xds.RegisterEndpointDiscoveryServiceServer(grpcServer, eds)
	log.Infof("starting emissary EndpointDiscoveryService service on port: %d", config.Port)
	go func() {
		err := grpcServer.Serve(lis)
		log.Errorf("error in eds grpc server", err)
		cancel()
	}()

	go func() {
		if err := http.ListenAndServe(fmt.Sprintf(":%d", config.AdminPort), nil); err != nil {
			panic(fmt.Sprintf("unable to bind to admin port %d, exiting.", config.AdminPort))
		}
	}()

	return grpcServer
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
		Port:         9001,
		AdminPort:    9002,
		PollInterval: 10 * time.Second,
		Dogstatsd:    defaultDogstatsdConfig(),
	}

	if len(config.Consul) != 0 && len(config.Docker.Host) != 0 {
		panic("use either -consul or -docker-host.")
	}

	var grpcServer *grpc.Server

	switch cmd, _ := conf.LoadWith(config, ld); cmd {
	case "eds":
		grpcServer = eds(ctx, cancel, config)
	default:
		panic("unknown command: " + cmd)
	}

	<-ctx.Done()
	log.Info("stopping emissary")

	if grpcServer != nil {
		log.Info("gracefully stopping gRPC server")
		grpcServer.GracefulStop()
		log.Info("grpcServer stopped")
	}
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
