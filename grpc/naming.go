// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.



package grpcsrvlb

import (
	"fmt"
	"time"

	"github.com/mwitkow/go-srvlb/srv"
	"google.golang.org/grpc/naming"
)

var (
	// MinimumRefreshInterval decides the maximum sleep time between SRV Lookups, otherwise controlled by TTL of records.
	MinimumRefreshInterval = 5 * time.Second
	// MaximumConsecutiveErrors identifies how many iterations of bad SRV Lookups to tolerate in a loop.
	MaximumConsecutiveErrors = 5
)

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
	targets, err := r.srvResolver.Lookup(target)
	if err != nil {
		return nil, fmt.Errorf("failed initial SRV resolution: %v", err)
	}
	return startNewWatcher(target, r.srvResolver, targets), nil
}

type updatesOrErr struct {
	updates []*naming.Update
	err     error
}

type watcher struct {
	domainName      string
	resolver        srv.Resolver
	existingTargets []*srv.Target
	next            chan *updatesOrErr
	close           chan bool
	erroredLoops    int
}

func startNewWatcher(domainName string, resolver srv.Resolver, targets []*srv.Target) *watcher {
	watcher := &watcher{
		domainName:      domainName,
		resolver:        resolver,
		existingTargets: targets,
		next:            make(chan *updatesOrErr),
	}
	go watcher.run()
	return watcher
}

func (w *watcher) run() {
	// First make sure that the initial read is an Add operation of the whole set.
	w.next <- &updatesOrErr{updates: targetsToUpdate(w.existingTargets, naming.Add)}
	erroredLoops := 0
	for true {
		timeToSleep := targetsMinTtl(w.existingTargets)
		select {
		case <-w.close:
			return
		case <-time.Tick(timeToSleep):
		}
		freshTargets, err := w.resolver.Lookup(w.domainName)
		if err != nil {
			erroredLoops += 1
			if erroredLoops > MaximumConsecutiveErrors {
				w.next <- &updatesOrErr{err: fmt.Errorf("SRV watcher failed after %d tries: %v", MaximumConsecutiveErrors, err)}
				return
			}
		}
		added := targetsSubstraction(freshTargets, w.existingTargets)
		deleted := targetsSubstraction(w.existingTargets, freshTargets)
		updates := targetsToUpdate(added, naming.Add)
		updates = append(updates, targetsToUpdate(deleted, naming.Delete)...)
		w.next <- &updatesOrErr{updates: updates}
		w.existingTargets = freshTargets
	}
}

// Next blocks until an update or error happens. It may return one or more
// updates. The first call should get the full set of the results. It should
// return an error if and only if Watcher cannot recover.
func (w *watcher) Next() ([]*naming.Update, error) {
	uE := <-w.next
	return uE.updates, uE.err
}

// Close closes the Watcher.
func (w *watcher) Close() {
	w.close <- true
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
