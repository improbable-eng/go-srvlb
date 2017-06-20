package grpcsrvlb

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/improbable-eng/go-srvlb/srv"
	"github.com/improbable-eng/go-srvlb/srv/mocks"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/naming"
)

func Test_NonBlockingBehaviour(t *testing.T) {
	c := clockwork.NewFakeClock()

	resolver := &mocks.Resolver{}
	w := startNewWatcher("testing.improbable.io", resolver, c)

	resolver.On("Lookup", "testing.improbable.io").Return([]*srv.Target{{
		DialAddr: "127.0.0.1",
		Ttl:      0,
	}}, nil)

	res, err := w.Next()
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, res[0], &naming.Update{Op: 0x0, Addr: "127.0.0.1"})

	res, err = w.Next()
	require.NoError(t, err)
	require.Len(t, res, 0)
}

func Test_BlockForTTL(t *testing.T) {
	c := clockwork.NewFakeClock()

	resolver := &mocks.Resolver{}
	w := startNewWatcher("testing.improbable.io", resolver, c)

	resTTL := 5 * time.Second
	resolver.On("Lookup", "testing.improbable.io").Return([]*srv.Target{{
		DialAddr: "127.0.0.1",
		Ttl:      resTTL,
	}}, nil)

	res, err := w.Next()
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, res[0], &naming.Update{Op: 0x0, Addr: "127.0.0.1"})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		res, err = w.Next()
		require.NoError(t, err)
		require.Len(t, res, 0)
		wg.Done()
	}()

	c.BlockUntil(1)
	c.Advance(resTTL)
	wg.Wait()
}

func Test_WatcherClosed(t *testing.T) {
	c := clockwork.NewFakeClock()

	resolver := &mocks.Resolver{}
	w := startNewWatcher("testing.improbable.io", resolver, c)

	w.Close()

	_, err := w.Next()
	require.Error(t, err, errClosed)
}

func Test_ResolverRetriesOnce(t *testing.T) {
	c := clockwork.NewFakeClock()

	resolver := &mocks.Resolver{}
	w := startNewWatcher("testing.improbable.io", resolver, c)

	resolver.On("Lookup", "testing.improbable.io").Return(nil, fmt.Errorf("datastore: concurrent transaction")).Once()

	resolver.On("Lookup", "testing.improbable.io").Return([]*srv.Target{{
		DialAddr: "127.0.0.1",
		Ttl:      0,
	}}, nil)

	res, err := w.Next()
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, res[0], &naming.Update{Op: 0x0, Addr: "127.0.0.1"})
}

func Test_ResolverRetriesFailsEventually(t *testing.T) {
	c := clockwork.NewFakeClock()

	resolver := &mocks.Resolver{}
	w := startNewWatcher("testing.improbable.io", resolver, c)
	resolver.On("Lookup", "testing.improbable.io").Return(nil, fmt.Errorf("datastore: concurrent transaction")).Times(MaximumConsecutiveErrors)

	_, err := w.Next()
	require.Error(t, err, "SRV watcher failed after 5 tries: datastore: concurrent transaction")
}
