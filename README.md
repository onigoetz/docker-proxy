# Docker Proxy

A small program that listens on an address or socket and redirects to a Docker Socket.

It prints information when a Docker Container is created or an image is pulled. (soon with push to InfluxDB)

## Starting

# TCP to TCP

```bash
go run main.go -listen localhost:9000 -target unix:/var/run/docker.sock
DOCKER_HOST=tcp://localhost:9000
```

# Unix socket to Unix socket

```bash
go run main.go -listen unix:./listen.sock -target unix:/var/run/docker.sock
```


# Mix TCP and Unix socket

```bash
go run main.go -listen unix:/tmp/listen.sock -target localhost:9001
```

### Try it locally 

```bash
DOCKER_HOST=tcp://localhost:9000
docker run -it --rm hello-world
```

Set DOCKER_HOST to the "listen" address of the proxy, it will print the container creations and image pulls.
