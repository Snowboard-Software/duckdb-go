package duckdb

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/duckdb/duckdb-go/v2/mapping"
	"github.com/stretchr/testify/require"
)

func TestWithNativeConnection(t *testing.T) {
	db := openDbWrapper(t, ``)
	defer closeDbWrapper(t, db)
	conn := openConnWrapper(t, db, context.Background())
	expected, err := ConnId(conn)
	require.NoError(t, err)
	callbackError := errors.New("callback sentinel")
	err = WithNativeConnection(conn, func(native mapping.Connection) error {
		var ctx mapping.ClientContext
		mapping.ConnectionGetClientContext(native, &ctx)
		defer mapping.DestroyClientContext(&ctx)
		require.Equal(t, expected, uint64(mapping.ClientContextGetConnectionId(ctx)))
		return callbackError
	})
	require.ErrorIs(t, err, callbackError)
	require.NoError(t, conn.Close())
	err = WithNativeConnection(conn, func(mapping.Connection) error {
		t.Fatal("closed connection reached native callback")
		return nil
	})
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.Error(t, WithNativeConnection(nil, func(mapping.Connection) error { return nil }))
	require.Error(t, WithNativeConnection(conn, nil))
}

func TestWithNativeConnectionSerializesCallbacks(t *testing.T) {
	db := openDbWrapper(t, ``)
	defer closeDbWrapper(t, db)
	conn := openConnWrapper(t, db, context.Background())
	defer closeConnWrapper(t, conn)
	var active atomic.Int32
	start := make(chan struct{})
	errors := make(chan error, 32)
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			errors <- WithNativeConnection(conn, func(native mapping.Connection) error {
				if active.Add(1) != 1 {
					t.Error("native callbacks overlapped on one physical connection")
				}
				defer active.Add(-1)
				var ctx mapping.ClientContext
				mapping.ConnectionGetClientContext(native, &ctx)
				mapping.DestroyClientContext(&ctx)
				return nil
			})
		}()
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

func TestWithNativeConnectionCallbackPanicDiscardsConnection(t *testing.T) {
	db := openDbWrapper(t, ``)
	defer closeDbWrapper(t, db)
	db.SetMaxOpenConns(1)
	conn := openConnWrapper(t, db, context.Background())
	require.PanicsWithValue(t, "native callback panic", func() {
		_ = WithNativeConnection(conn, func(mapping.Connection) error {
			panic("native callback panic")
		})
	})
	_, err := conn.ExecContext(context.Background(), "SELECT 1")
	require.ErrorIs(t, err, sql.ErrConnDone)
	var result int
	require.NoError(t, db.QueryRow("SELECT 42").Scan(&result))
	require.Equal(t, 42, result)
}
