# Emissary [![CircleCI](https://circleci.com/gh/segmentio/emissary.svg?style=svg)](https://circleci.com/gh/segmentio/emissary) [![GoDoc](https://godoc.org/github.com/segmentio/emissary?status.svg)](https://godoc.org/github.com/segmentio/emissary)

Emissary is a data plane for Envoy https://github.com/envoyproxy/data-plane-api/blob/master/XDS_PROTOCOL.md

## Resolvers

Emissary can currently use Consul or the Docker API as resolvers.

### Consul Resolver

The Consul resolver will resolve the service to a list of endpoints using the name of the configured virtual host.

### Docker Resolver

The Docker resolver will be looking for a label to identify the destination containers for Envoy.
By default the label is `emissary.service_name` but can be override in the configuration.

## Development
```
dep ensure
make
```

make will build a single executable `emissary` and tag a docker image `emissary:latest`

Run the tests using make:

```
$ make test
# For more verbosity (`Q=` trick applies to all targets)
$ make test Q=
```

## Examples

### Using Emissary with Consul

[examples/eds_grpc](examples/eds_grpc) and [examples/eds_az_aware_grpc](examples/eds_az_aware_grpc)
are two examples on how to use Emissary with Consul.

```
cd examples/eds_grpc
make
docker-compose up
````

```
cd examples/eds_az_aware_grpc
make
docker-compose up
````

Each example starts 2 "server" containers with a trivial http server listening on port 8077. It then starts an envoy
instance to serve as the loadbalancer for the upstream server cluster. Additionally we start a consul and registrator containers.
Finally we start a client which connects to envoy

The examples share the same containers so if you start and stop different examples you may need to clean your stopped containers

```
docker rm $(docker ps -qa --no-trunc --filter "status=exited")
```

### Using Emissary with Docker API

You can use Emissary to discover local Docker containers and load balance traffic to those via Envoy.
You can find a example for this pattern in [examples/docker_resolver](examples/docker_resolver).

```
cd examples/docker_resolver
make up
```
