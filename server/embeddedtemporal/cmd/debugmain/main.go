package main
import (
  "fmt"
  "time"
  "net"
  "path/filepath"
  et "github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
)
func main(){
  db := filepath.Join("/tmp","et-debug.db")
  srv, err := et.NewServer(et.Options{
    FrontendIP: "127.0.0.1",
    FrontendPort: 17236,
    DatabaseFile: db,
    LogLevel: "error",
    Namespaces: []string{"main-test"},
  })
  if err != nil { panic(err) }
  if err := srv.Start(); err != nil { panic(err) }
  fmt.Println("server started")
  // try raw TCP dial
  for i:=0;i<20;i++{
    conn, err := net.DialTimeout("tcp","127.0.0.1:17888", time.Second)
    if err==nil { fmt.Println("tcp ok"); conn.Close(); break }
    fmt.Println("dial err:", err)
    time.Sleep(time.Second)
  }
  // try Temporal client
  c, err := et.NewClient(et.ClientOptions{HostPort: "127.0.0.1:17236", Namespace: "main-test"})
  if err != nil { fmt.Println("NewClient error:", err) } else { fmt.Println("client ok"); c.Close() }
  time.Sleep(2*time.Second)
  _ = srv.Stop()
}
