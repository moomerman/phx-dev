package adapter

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"

	"github.com/moomerman/phx-dev/multiproxy"
)

type ProxyAdapter struct {
	Host  string
	Dir   string
	Port  string
	proxy *multiproxy.MultiProxy
}

func CreateProxyAdapter(host, dir, port string) (Adapter, error) {
	return &ProxyAdapter{
		Host: host,
		Dir:  dir,
		Port: port,
	}, nil
}

func (d *ProxyAdapter) Stop() error          { return nil }
func (d *ProxyAdapter) Command() *exec.Cmd   { return nil }
func (d *ProxyAdapter) WriteLog(w io.Writer) {}

func (d *ProxyAdapter) Start() error {
	// TODO: read proxy host/port from file
	addr := "http://127.0.0.1:" + d.Port
	fmt.Println("[proxy]", d.Host, "starting proxy to", addr)
	d.proxy = multiproxy.NewProxy(addr, d.Host)
	return nil
}

func (d *ProxyAdapter) Serve(w http.ResponseWriter, r *http.Request) {
	fmt.Println("[proxy]", fullURL(r), "->", d.proxy.URL)
	d.proxy.Proxy(w, r)
}