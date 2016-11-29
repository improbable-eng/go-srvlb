// Copyright (c) Improbable Worlds Ltd, All Rights Reserved

package srv

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// NewGoResolver is a resolver that uses net-package LookupSRV.
// It doesn't support TTL expiration and as such returns a dummy TTL.
// It's also very inefficient for large targets.
func NewGoResolver(dummyTtl time.Duration) Resolver {
	return &golangResolver{ttl: dummyTtl}
}

type golangResolver struct {
	ttl time.Duration
}

func (r *golangResolver) Lookup(domainName string) ([]*Target, error) {
	_, srvs, err := net.LookupSRV("", "", domainName)
	if err != nil {
		return nil, err
	}
	ret := []*Target{}
	// This is naive and will cause a lot of latency.
	for _, s := range srvs {
		addrs, err := net.LookupHost(s.Target)
		if err != nil {
			continue
		}
		ret = append(ret, &Target{Ttl: r.ttl, DialAddr: fmt.Sprintf("%v:%v", addrs[0], s.Port)})
	}
	if len(ret) == 0 {
		return nil, errors.New("failed resolving hostnames for SRV entries")
	}
	return ret, nil
}
