package emissary

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDockerResolver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Write([]byte(`[
  {
    "Id": "b084550d7373af8628b44173121dc162a377c499896e8b1704cb10d9f80cde5d",
    "Names": [
      "/docker_resolver_bar_1"
    ],
    "Image": "docker_resolver_bar",
    "ImageID": "sha256:5232a6f8826bc5c6bbc1da9683a4e2e2490abb581cdf50a8fc93ba1eabfef2ca",
    "Command": "/server bar 3000",
    "Created": 1547259718,
    "Ports": [
      {
        "IP": "0.0.0.0",
        "PrivatePort": 3000,
        "PublicPort": 3002,
        "Type": "tcp"
      }
    ],
    "Labels": {
      "com.docker.compose.config-hash": "edef74d91523c043eca8b94f3282ca383d2a7339cbf1fb1152ce8731e1dd2093",
      "com.docker.compose.container-number": "1",
      "com.docker.compose.oneoff": "False",
      "com.docker.compose.project": "docker_resolver",
      "com.docker.compose.service": "bar",
      "com.docker.compose.version": "1.21.2",
      "emissary.service_name": "foobar-api"
    },
    "State": "running",
    "Status": "Up Less than a second",
    "HostConfig": {
      "NetworkMode": "docker_resolver_envoymesh"
    },
    "NetworkSettings": {
      "Networks": {
        "docker_resolver_envoymesh": {
          "IPAMConfig": null,
          "Links": null,
          "Aliases": null,
          "NetworkID": "1512cc3b251a1d4f1441efeaa3322929ca3e141d7fdb6529bd406186c88f7c18",
          "EndpointID": "b9216f159aa89cafa7f61b87592d4012e9630484551ee8bf7a7bd184221e53c2",
          "Gateway": "172.20.0.1",
          "IPAddress": "172.20.0.3",
          "IPPrefixLen": 16,
          "IPv6Gateway": "",
          "GlobalIPv6Address": "",
          "GlobalIPv6PrefixLen": 0,
          "MacAddress": "02:42:ac:14:00:03",
          "DriverOpts": null
        }
      }
    },
    "Mounts": []
  },
  {
    "Id": "d0b5d1b6e35c3560a8a4277376ad99be43b36870f62cf78b551594cdd0bd237a",
    "Names": [
      "/docker_resolver_foo_1"
    ],
    "Image": "docker_resolver_foo",
    "ImageID": "sha256:5232a6f8826bc5c6bbc1da9683a4e2e2490abb581cdf50a8fc93ba1eabfef2ca",
    "Command": "/server foo 3000",
    "Created": 1547259718,
    "Ports": [
      {
        "IP": "0.0.0.0",
        "PrivatePort": 3000,
        "PublicPort": 3001,
        "Type": "tcp"
      }
    ],
    "Labels": {
      "com.docker.compose.config-hash": "aecf5053500470d21742be5e6c6d89e64ac802f6178e56100f7508b307635431",
      "com.docker.compose.container-number": "1",
      "com.docker.compose.oneoff": "False",
      "com.docker.compose.project": "docker_resolver",
      "com.docker.compose.service": "foo",
      "com.docker.compose.version": "1.21.2",
      "emissary.service_name": "foobar-api"
    },
    "State": "running",
    "Status": "Up 2 seconds",
    "HostConfig": {
      "NetworkMode": "docker_resolver_envoymesh"
    },
    "NetworkSettings": {
      "Networks": {
        "docker_resolver_envoymesh": {
          "IPAMConfig": null,
          "Links": null,
          "Aliases": null,
          "NetworkID": "1512cc3b251a1d4f1441efeaa3322929ca3e141d7fdb6529bd406186c88f7c18",
          "EndpointID": "b409ad73d54e1287d4efe235dae9b306550724a736a828b177322d710c69d87c",
          "Gateway": "172.20.0.1",
          "IPAddress": "172.20.0.2",
          "IPPrefixLen": 16,
          "IPv6Gateway": "",
          "GlobalIPv6Address": "",
          "GlobalIPv6PrefixLen": 0,
          "MacAddress": "02:42:ac:14:00:02",
          "DriverOpts": null
        }
      }
    },
    "Mounts": []
  }
]
`))
	}))

	defer server.Close()

	resolver := DockerResolver{
		Client: DockerClient{
			Host: server.URL[7:],
		},
	}

	endpoints, err := resolver.Lookup(context.TODO(), "foobar-api")
	if err != nil {
		t.Fatal(err)
	}

	if len(endpoints) != 2 {
		t.Error("unexpected number of endpoints")
		t.Log("got: ", len(endpoints))
	}

	addr := "0.0.0.0:3002"
	if endpoints[0].Addr.String() != addr {
		t.Error("unexpected endpoint")
		t.Log("got: ", endpoints[0].Addr)
		t.Log("expected: ", addr)
	}
}

func TestDockerHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Write([]byte(`{
  "ID": "7TRN:IPZB:QYBB:VPBQ:UMPP:KARE:6ZNR:XE6T:7EWV:PKF4:ZOJD:TPYS",
  "SystemStatus": [
    [
      "Role",
      "primary"
    ],
    [
      "State",
      "Healthy"
    ],
    [
      "Strategy",
      "spread"
    ]
  ],
  "SecurityOptions": [
    "name=apparmor",
    "name=seccomp,profile=default",
    "name=selinux",
    "name=userns"
  ]
}
`))
	}))

	defer server.Close()

	resolver := DockerResolver{
		Client: DockerClient{
			Host: server.URL[7:],
		},
	}

	if !resolver.Healthy(context.TODO()) {
		t.Error("docker should be healthy")
	}
}
