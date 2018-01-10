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
	"github.com/jpillora/backoff"
	"google.golang.org/grpc/naming"
)

const (
	// NoRetryLimit specifies that there is no limit to number of srv lookup failures.
	NoRetryLimit = -1
)

var (
	// MinimumRefreshInterval decides the maximum sleep time between SRV Lookups, otherwise controlled by TTL of records.
	MinimumRefreshInterval = 5 * time.Second

	errClosed = errors.New("go-srvlb: closed")
)

// resolver implements the naming.Resolver interface from gRPC.
type resolver struct {
	srvResolver          srv.Resolver
	maxConsecutiveErrors int
	retryBackoff         *backoff.Backoff
}

type SrvResolverOptions func(*resolver)

// MaximumConsecutiveErrors identifies how many consecutive iterations of bad SRV Lookups to tolerate in a loop.
// After this limit is reached all the srv resolutions will stop.
// Default value is -1. -1 means there is no limit.
func WithMaximumConsecutiveErrors(maxErr int) SrvResolverOptions {
	return func(resolver *resolver) {
		resolver.maxConsecutiveErrors = maxErr
	}
}

// WithRetryBackoff sets or clears the backoff when retrying after lookup failures.
// By default, an exponential backoff with a maximum of MinimumRefreshInterval is used.
// Pass nil to clear backoff, causing instant retries.
func WithRetryBackoff(retryBackoff *backoff.Backoff) SrvResolverOptions {
	return func(resolver *resolver) {
		resolver.retryBackoff = retryBackoff
	}
}

// New creates a gRPC naming.Resolver that is backed by an SRV lookup resolver.
func New(srvResolver srv.Resolver, options ...SrvResolverOptions) naming.Resolver {
	res := &resolver{
		srvResolver:          srvResolver,
		maxConsecutiveErrors: NoRetryLimit,
		retryBackoff:         &backoff.Backoff{Max: MinimumRefreshInterval},
	}
	for _, opt := range options {
		opt(res)
	}
	return res
}

// Resolve creates a Watcher for target.
func (r *resolver) Resolve(target string) (naming.Watcher, error) {
	return startNewWatcher(target, r.srvResolver, clockwork.NewRealClock(), r.maxConsecutiveErrors, r.retryBackoff), nil
}

type updatesOrErr struct {
	updates []*naming.Update
	err     error
}

type watcher struct {
	domainName           string
	resolver             srv.Resolver
	existingTargets      []*srv.Target
	erroredLoops         int
	lastFetch            time.Time
	closed               int32
	clock                clockwork.Clock
	mutex                sync.Mutex
	maxConsecutiveErrors int
	retryBackoff         *backoff.Backoff
}

func startNewWatcher(domainName string, resolver srv.Resolver, clock clockwork.Clock, maxErrors int, retryBackoff *backoff.Backoff) *watcher {
	if retryBackoff != nil {
		backoffCopy := *retryBackoff
		retryBackoff = &backoffCopy
	}
	watcher := &watcher{
		domainName:           domainName,
		resolver:             resolver,
		lastFetch:            time.Unix(0, 0),
		clock:                clock,
		maxConsecutiveErrors: maxErrors,
		retryBackoff:         retryBackoff,
	}
	return watcher
}

// Next blocks until an update or error happens. It may return one or more
// updates. The first call should get the full set of the results. It should
// return an error if and only if Watcher cannot recover.
func (w *watcher) Next() ([]*naming.Update, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	nextFetchTime := w.lastFetch.Add(targetsMinTtl(w.existingTargets))
	timeUntilFetch := nextFetchTime.Sub(w.clock.Now())
	if timeUntilFetch > 0 {
		w.clock.Sleep(timeUntilFetch)
	}
	if w.retryBackoff != nil {
		w.retryBackoff.Reset()
	}

	var lastErr error
	consecutiveErrors := 0
	for {
		if atomic.LoadInt32(&w.closed) == 1 {
			return nil, errClosed
		}
		freshTargets, err := w.resolver.Lookup(w.domainName)
		if err != nil {
			lastErr = err
			consecutiveErrors += 1
			if w.maxConsecutiveErrors != NoRetryLimit && consecutiveErrors >= w.maxConsecutiveErrors {
				break
			}
			if w.retryBackoff != nil {
				w.clock.Sleep(w.retryBackoff.Duration())
			}
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
	return nil, fmt.Errorf("SRV watcher failed after %d tries: %v", w.maxConsecutiveErrors, lastErr)
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
