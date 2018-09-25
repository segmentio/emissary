package main

import (
	"context"
	"fmt"
	"github.com/apex/log"
	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/pkg/errors"
	"github.com/segmentio/conf"
	"github.com/segmentio/consul-go"
	"github.com/segmentio/emissary"
	"github.com/segmentio/events"
	"google.golang.org/grpc"
	"net"
	"os"
	"syscall"
)

type emissaryConfig struct {
	Port   int    `conf:"port"`
	Consul string `conf:"consul"`
}

func eds(ctx context.Context, client *consul.Client, config *emissaryConfig) {
	defer ctx.Done()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		log.WithError(errors.Wrap(err, "starting EndpointDiscoveryService"))
		return
	}

	grpcServer := grpc.NewServer()
	xds.RegisterEndpointDiscoveryServiceServer(grpcServer, emissary.NewEdsService(client))
	log.Infof("starting emissary EndpointDiscoveryService service on port: %d", config.Port)
	grpcServer.Serve(lis)
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
		Port:   9001,
		Consul: "localhost:8500",
	}

	switch cmd, _ := conf.LoadWith(config, ld); cmd {
	case "eds":
		client := &consul.Client{Address: config.Consul}
		eds(ctx, client, config)
	default:
		panic("unknown command: " + cmd)
	}
}
