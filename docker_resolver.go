package emissary

import (
	"context"
	"fmt"
	"net"
)

// DockerResolver implements the Resolver interface.
type DockerResolver struct {
	Client DockerClient
}

// DockerLookupPort is used to lookup the Ip and Port from the containers metadata.
var DockerServicePort int = 3000

// DockerLookupLabel is used to filter containers.
var DockerServiceLabel string = "emissary.service_name"

// Lookup for the service in docker from the docker api. Lookup with look for the label DockerServiceLabel
// in the container metadata.
//
// Currently no tag will be set in the resulted endpoints.
func (d *DockerResolver) Lookup(ctx context.Context, service string) ([]Endpoint, error) {
	containers, err := d.Client.listContainers()
	if err != nil {
		return nil, err
	}

	var endpoints []Endpoint
	for _, container := range containers {
		if svc, ok := container.Labels[DockerServiceLabel]; ok && svc == service {
			var containerIP string
			for _, setting := range container.NetworkSettings.Networks {
				if setting.IPAddress != "" {
					containerIP = setting.IPAddress
					break
				}
			}

			addr, err := parseAddr(containerIP, DockerServicePort)
			if err != nil {
				continue
			}

			//TODO: do we need to set some tags ?
			endpoints = append(endpoints, Endpoint{
				Addr: addr,
			})
		}
	}

	return endpoints, nil
}

func (d *DockerResolver) Healthy(ctx context.Context) bool {
	state, err := d.Client.getStatus("State")
	if err != nil {
		return false
	}
	return state == "Healthy"
}

func parseAddr(hostIp string, port int) (*net.TCPAddr, error) {
	ip := net.ParseIP(hostIp)
	if ip == nil {
		return nil, fmt.Errorf("invalid hostIp %s", hostIp)
	}

	return &net.TCPAddr{IP: ip, Port: port}, nil
}
