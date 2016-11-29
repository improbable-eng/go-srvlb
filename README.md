# HTTP CONNECT tunneling Go Dialer

[![Go Report Card](https://goreportcard.com/badge/github.com/mwitkow/go-srvlb)](http://goreportcard.com/report/mwitkow/go-srvlb)
[![GoDoc](http://img.shields.io/badge/GoDoc-Reference-blue.svg)](https://godoc.org/github.com/mwitkow/go-srvlb)
[![Apache 2.0 License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A gRPC [`naming.Resolver`](https://godoc.org/google.golang.org/grpc/naming) that uses [DNS SRV](https://en.wikipedia.org/wiki/SRV_record).
This allows you to do simple client-side Round Robin load balancing of gRPC requests.

# Usage

```go
rr := grpc.RoundRobin(grpcsrvlb.New(srv.NewGoResolver(2 * time.Second)))
conn, err := grpc.Dial("grpc.my_service.my_cluster.internal.example.com", grpc.WithBalancer(rr))
```

This will resolve the DNS SRV address `grpc.my_service.my_cluster.internal.example.com` using the Golang DNS resolver
with an assumed TTL of 2 seconds and use that as a set of backends for the gRPC `RoundRobin` policy. From this point on
all requests on the `conn` (reusable across gRPC clients) will be load balanced to a set of these backends.

# Status

This is *alpha* software. It should work, but key components are missing:

 * [ ] unit tests
 * [ ] integration tests with gRPC
 * [ ] `srv.Resolver` implementation that is concurrent and respects `TTL`, see [miekg/dns](https://github.com/miekg/dns)


# License

 `go-srvlb` is released under the Apache 2.0 license. See the [LICENSE](LICENSE) file for details.

