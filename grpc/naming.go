// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package grpcsrvlb

import (
	"fmt"
	"time"

	"errors"
	"sync"
	"sync/atomic"

	"github.com/improbable-eng/go-srvlb/srv"
	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc/naming"
)

var (
	// MinimumRefreshInterval decides the maximum sleep time between SRV Lookups, otherwise controlled by TTL of records.
	MinimumRefreshInterval = 5 * time.Second
	// MaximumConsecutiveErrors identifies how many iterations of bad SRV Lookups to tolerate in a loop.
	MaximumConsecutiveErrors = 5
)

var errClosed = errors.New("go-srvlb: closed")

// resolver implements the naming.Resolver interface from gRPC.
type resolver struct {
	srvResolver srv.Resolver
}

// New creates a gRPC naming.Resolver that is backed by an SRV lookup resolver.
func New(srvResolver srv.Resolver) naming.Resolver {
	return &resolver{srvResolver: srvResolver}
}

// Resolve creates a Watcher for target.
func (r *resolver) Resolve(target string) (naming.Watcher, error) {
	return startNewWatcher(target, r.srvResolver, clockwork.NewRealClock()), nil
}

type updatesOrErr struct {
	updates []*naming.Update
	err     error
}

type watcher struct {
	domainName      string
	resolver        srv.Resolver
	existingTargets []*srv.Target
	erroredLoops    int
	lastFetch       time.Time
	closed          int32
	clock           clockwork.Clock
	mutex           sync.Mutex
}

func startNewWatcher(domainName string, resolver srv.Resolver, clock clockwork.Clock) *watcher {
	watcher := &watcher{
		domainName: domainName,
		resolver:   resolver,
		lastFetch:  time.Unix(0, 0),
		clock:      clock,
	}
	return watcher
}

// Next blocks until an update or error happens. It may return one or more
// updates. The first call should get the full set of the results. It should
// return an error if and only if Watcher cannot recover.
func (w *watcher) Next() ([]*naming.Update, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if atomic.LoadInt32(&w.closed) == 1 {
		return nil, errClosed
	}
	nextFetchTime := w.lastFetch.Add(targetsMinTtl(w.existingTargets))
	timeUntilFetch := nextFetchTime.Sub(w.clock.Now())

	if timeUntilFetch > 0 {
		w.clock.Sleep(timeUntilFetch)
	}

	var lastErr error
	for i := 0; i < MaximumConsecutiveErrors; i++ {
		freshTargets, err := w.resolver.Lookup(w.domainName)
		if err != nil {
			lastErr = err
			continue
		}
		added := targetsSubstraction(freshTargets, w.existingTargets)
		deleted := targetsSubstraction(w.existingTargets, freshTargets)
		updates := targetsToUpdate(added, naming.Add)
		updates = append(updates, targetsToUpdate(deleted, naming.Delete)...)
		w.existingTargets = freshTargets
		w.lastFetch = w.clock.Now()
		return updates, nil
	}
	return nil, fmt.Errorf("SRV watcher failed after %d tries: %v", MaximumConsecutiveErrors, lastErr)
}

// Close closes the Watcher.
func (w *watcher) Close() {
	atomic.StoreInt32(&w.closed, 1)
}

func targetsToUpdate(targets []*srv.Target, op naming.Operation) []*naming.Update {
	ret := []*naming.Update{}
	for _, t := range targets {
		ret = append(ret, &naming.Update{Addr: t.DialAddr, Op: op})
	}
	return ret
}

func targetsMinTtl(targets []*srv.Target) time.Duration {
	ret := MinimumRefreshInterval
	for _, t := range targets {
		if t.Ttl < ret {
			ret = t.Ttl
		}
	}
	return ret
}

// targetsSubstraction calculates a set difference of `from / to` on target sets.
func targetsSubstraction(from []*srv.Target, to []*srv.Target) []*srv.Target {
	ret := []*srv.Target{}
	for _, f := range from {
		exists := false
		for _, t := range to {
			if t.DialAddr == f.DialAddr {
				exists = true
				break
			}
		}
		if !exists {
			ret = append(ret, f)
		}
	}
	return ret
}
