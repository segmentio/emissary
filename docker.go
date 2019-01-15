package emissary

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const defaultDockerHost string = "unix:///var/run/docker.sock"

// DockerClient is meant to be used in DockerResolver.
type DockerClient struct {
	Host string
}

func (c *DockerClient) listContainers() (containers []dockerContainer, err error) {
	err = c.get("/containers/json", &containers)
	return
}

func (c *DockerClient) get(path string, ret interface{}) (err error) {
	var req *http.Request
	var res *http.Response

	if c.Host == "" {
		c.Host = defaultDockerHost
	}

	dialContext := func(ctx context.Context, _, _ string) (net.Conn, error) {
		network, address := dockerNetworkAddress(c.Host)
		return (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, network, address)
	}

	transport := &http.Transport{
		DialContext:            dialContext,
		DisableKeepAlives:      true,
		DisableCompression:     true,
		ResponseHeaderTimeout:  5 * time.Second,
		ExpectContinueTimeout:  5 * time.Second,
		MaxResponseHeaderBytes: 1024 * 1024,
	}
	defer transport.CloseIdleConnections()

	if req, err = http.NewRequest(http.MethodGet, "http://docker"+path, nil); err != nil {
		return
	}

	if res, err = transport.RoundTrip(req); err != nil {
		return
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("%s: %s", req.URL, res.Status)
		return
	}

	err = json.NewDecoder(res.Body).Decode(ret)
	return
}

func (c *DockerClient) getStatus(key string) (string, error) {
	info := dockerInfo{}
	err := c.get("/info", &info)
	if err != nil {
		return "", err
	}

	for _, s := range info.SystemStatus {
		if len(s) != 2 {
			continue
		}

		if s[0] == key {
			return s[1], nil
		}
	}

	return "", fmt.Errorf("%s status not found", key)
}

type dockerContainer struct {
	Image           dockerImage
	NetworkSettings dockerNetworkSettings
	Labels          dockerLabels
	Ports           []dockerPort
}

type dockerLabels map[string]string

type dockerPort struct {
	IP          string
	PrivatePort int
	PublicPort  int
	Type        string
}

type dockerNetworkSettings struct {
	Networks map[string]dockerNetwork
}

type dockerNetwork struct {
	IPAMConfig dockerIPAMConfig
	IPAddress  string
}

type dockerIPAMConfig struct {
	IPv4Address string
	IPv6Address string
}

type dockerInfo struct {
	SystemStatus [][]string
}

type dockerImage string

func (image dockerImage) repo() string {
	repo, _, _ := image.parts()
	return repo
}

func (image dockerImage) name() string {
	_, name, _ := image.parts()
	return name
}

func (image dockerImage) version() string {
	_, _, version := image.parts()
	return version
}

func (image dockerImage) parts() (repo, name, version string) {
	s := string(image)
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		name = s
	} else {
		name, version = s[:i], s[i+1:]
	}
	j := strings.LastIndexByte(name, '/')
	if j >= 0 {
		repo, name = name[:j], name[j+1:]
	}
	return
}

func dockerNetworkAddress(host string) (network, address string) {
	if len(host) != 0 {
		if i := strings.Index(host, "://"); i >= 0 {
			network, address = host[:i], host[i+3:]
		} else if address = host; strings.HasPrefix(address, "/") {
			network = "unix"
		} else {
			network = "tcp"
		}
		if network == "tcp" {
			if _, port, _ := net.SplitHostPort(address); len(port) == 0 {
				address = net.JoinHostPort(address, "2376") // default docker port
			}
		}
	}
	return
}

func serviceName(c dockerContainer) string {
	name, ok := c.Labels["com.amazonaws.ecs.container-name"]
	if ok {
		return name
	}

	return string(c.Image)
}
