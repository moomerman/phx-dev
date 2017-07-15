package adapter

import (
	"io"
	"net"
	"net/http"
	"os/exec"
)

type Adapter interface {
	Command() *exec.Cmd
	Start() error
	Stop() error
	WriteLog(io.Writer)
	Serve(w http.ResponseWriter, r *http.Request)
}

func findAvailablePort() (string, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}
	l.Close()

	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return "", err
	}

	return port, nil
}
