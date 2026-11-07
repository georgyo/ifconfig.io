
# ifconfig.io

[![Build Status](https://drone.io/github.com/georgyo/ifconfig.io/status.png)](https://drone.io/github.com/georgyo/ifconfig.io/latest)

Inspired by ifconfig.me, but designed for pure speed. A single server can do 18,000 requests per seconds while only consuming 50megs of ram.

# Contents

- [Short Summery](#short-summery)
- [Deployment](#deployment)
  - [Docker Compose](#docker-compose)
  - [ENVs](#envs)

# Short Summary

The server is built directly on the Go standard library's net/http. The command line endpoints (`/`, `/ip`, `/all.json`, ...) are written so they do not allocate on each request, keeping the GC happy. Run `go test -bench .` to see per-endpoint numbers.

Tested to handle 10,000 clients doing 90,000 requests persecond on modest hardware with an average response time of 42ms. Easily servicing over 5 million requests in a minute. (Updated June, 2022)

[![LoadTest](http://i.imgur.com/0vJYumD.png)](https://loader.io/reports/f1e9a7dd516ac0472351e5e0c83b0787/results/a055e51ff317cdf8a688b25e9c0e4147#response_details)

# Deployment

You can use the source code directly to deploy your own server. You can also use Docker and Docker Compose.

## Docker-Compose

Here is a sample docker-compose file:

``` bash
version: "3.4"

services:
  ifconfig:
    image: georgyo/ifconfig.io
    ports:
      - 8080:8080
```

Some other compose files:

``` bash
version: "3.4"

services:
  ifconfig:
    image: georgyo/ifconfig.io
    ports:
      - 8080:8080
    environment:
      HOSTNAME: "ifconfig.io"
```

``` bash
version: "3.4"

services:
  ifconfig:
    image: georgyo/ifconfig.io
    ports:
      - 8080:8080
    env_file:
      - ./.env
```

## ENVs

This project offers you some customizability over what you show to user and ect. Here is list of all possible environment variable that you can pass to your instance, with their default values.

``` bash
HOSTNAME="ifconfig.io" # Text address shown to user at header
CMD_PROTOCOL="" # Request protocol for curl commands
HOST=""
PORT="8080"
FORWARD_IP_HEADER="CF-Connecting-IP" # Request header to get IP from
COUNTRY_CODE_HEADER="CF-IPCountry" # Request header to get country code from
TLS="0"
TLSPORT="8443"
TLSCERT="/opt/ifconfig/.cf/ifconfig.io.crt"
TLSKEY="/opt/ifconfig/.cf/ifconfig.io.key"
REVERSE_DNS_TIMEOUT="300ms" # Max time a request waits for a reverse DNS lookup
REVERSE_DNS_CACHE_SIZE="100000" # Number of reverse DNS results kept in memory
TRUSTED_PROXIES="cloudflare,private" # Peers allowed to set FORWARD_IP_HEADER
```

`TRUSTED_PROXIES` takes a comma separated list of CIDRs, IP addresses, and the
keywords `cloudflare` (Cloudflare's published ranges), `private` (loopback,
RFC 1918, CGNAT, link-local and ULA ranges) and `*` (everyone).

`FORWARD_IP_HEADER` is ignored unless the connection comes from one of the
`TRUSTED_PROXIES`, so clients cannot spoof their address by sending the header
themselves. Set `TRUSTED_PROXIES="*"` to restore the old behaviour of trusting
the header from anyone.

The Go garbage collector is tuned for this workload at startup: `GOGC`
defaults to 200, and when running under a cgroup memory limit `GOMEMLIMIT`
defaults to 90% of that limit. Setting either environment variable overrides
the default.
