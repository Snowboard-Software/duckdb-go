package duckdb

import (
	"database/sql"
	"fmt"

	"github.com/duckdb/duckdb-go/v2/mapping"
)

// WithNativeConnection gives a trusted native integration temporary access to
// the driver's DuckDB handle under database/sql's connection lock. Neither the
// handle nor objects that retain it may escape fn. fn must not close the handle,
// call back through c, or run concurrently with outstanding rows/statements.
// This exposes no native operation to SQL clients.
func WithNativeConnection(c *sql.Conn, fn func(mapping.Connection) error) error {
	if c == nil || fn == nil {
		return fmt.Errorf("native connection access requires a connection and callback")
	}
	return c.Raw(func(raw any) error {
		conn, ok := raw.(*Conn)
		if !ok {
			return fmt.Errorf("native connection access requires the DuckDB driver")
		}
		if conn.closed {
			return errClosedCon
		}
		return fn(conn.conn)
	})
}
