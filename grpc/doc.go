package grpcsrvlb

/*
This package implements a gRPC `naming` resolver for client-side load balancing of DNS SRV backends.

Usage:

  rr := grpc.RoundRobin(grpcsrvlb.New(srv.NewGoResolver(2 * time.Second)))
  conn, err := grpc.Dial("grpc.my_service.my_cluster.internal.example.com", grpc.WithBalancer(rr))

*/
