# Emissary [![CircleCI](https://ci.segment.com/gh/segmentio/emissary.svg?style=svg&circle-token=e31f23668625c3449fe71c8b582ab33191190a50)](https://ci.segment.com/gh/segmentio/emissary)

emissary is a consul backed xDS for Envoy https://github.com/envoyproxy/data-plane-api/blob/master/XDS_PROTOCOL.md

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

The examples directory currently has two examples you can run locally with docker-compose

* eds_grpc
* eds_az_aware_grpc

```
cd eds_grpc
make
docker-compose up
````

```
cd eds_az_aware_grpc
make
docker-compose up
````

Each example starts 2 "server" containers with a trivial http server listening on port 8077. It then starts an envoy
instance to serve as the loadbalancer for the upstream server cluster. Additionally we start a consul and registrator containers.
Finally we start a client which connects to envoy

The examples share the same containers so if you start and stop different examples you may need to clean your stopped containers

```docker rm $(docker ps -qa --no-trunc --filter "status=exited")```
