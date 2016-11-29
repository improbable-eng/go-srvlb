// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package srv

import "time"

// Resolver is an implementation of a DNS SRV resolver for a domain.
type Resolver interface {
	Lookup(domainName string) ([]*Target, error)
}

// Target is a resolved backend behind an SRV address pool.
type Target struct {
	DialAddr string
	Ttl      time.Duration
}
