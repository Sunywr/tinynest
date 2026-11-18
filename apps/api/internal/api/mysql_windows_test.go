//go:build windows

package api

import (
 "context"
 "net"
 "os"
 "github.com/go-sql-driver/mysql"
)

// Windows CI can exercise the same MySQL queries on an isolated loopback TCP
// instance. Production continues to use the Unix socket transport.
func init(){if address:=os.Getenv("TINYNEST_TEST_MYSQL_TCP");address!=""{mysql.RegisterDialContext("unix",func(ctx context.Context,_ string)(net.Conn,error){return (&net.Dialer{}).DialContext(ctx,"tcp",address)})}}
