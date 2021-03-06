package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"

	zadapter "github.com/moomerman/zap/adapter"
	"github.com/moomerman/zap/rproxy"
	"github.com/puma/puma-dev/linebuffer"
	"github.com/vektra/errors"
)

// Config holds the server configuration
type Config struct {
	Name            string
	Scheme          string
	Host            string
	Dir             string
	EnvPortName     string
	ShellCommand    string
	RestartPatterns []*regexp.Regexp
}

// New returns a new server adapter
func New(config *Config) zadapter.Adapter {
	return &adapter{
		Name:            config.Name,
		Scheme:          config.Scheme,
		Host:            config.Host,
		Dir:             config.Dir,
		EnvPortName:     config.EnvPortName,
		ShellCommand:    config.ShellCommand,
		RestartPatterns: config.RestartPatterns,
	}
}

type adapter struct {
	sync.Mutex

	Name            string
	Scheme          string
	Host            string
	Dir             string
	Port            string
	Command         string
	EnvPortName     string           `json:",omitempty"`
	RestartPatterns []*regexp.Regexp `json:",omitempty"`
	BootLog         string
	Pid             int
	ShellCommand    string

	stateMu    sync.Mutex
	state      zadapter.Status
	cmd        *exec.Cmd
	proxiesMu  sync.Mutex
	proxies    map[string]*rproxy.ReverseProxy
	stdout     io.Reader
	log        linebuffer.LineBuffer
	cancelChan chan struct{}
}

// Start starts the application
func (a *adapter) Start() error {
	a.Lock()
	defer a.Unlock()
	if a.state == zadapter.StatusStopping || a.state == zadapter.StatusRunning {
		return nil
	}

	log.Println("[app]", a.Host, "START")
	return a.start()
}

// Stop stops the application
func (a *adapter) Stop(reason error) error {
	a.Lock()
	defer a.Unlock()
	if a.state == zadapter.StatusStopping || a.state == zadapter.StatusStopped {
		return nil
	}

	log.Println("[app]", a.Host, "STOP", reason)
	return a.stop()
}

// Status returns the status of the adapter
func (a *adapter) Status() zadapter.Status {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	return a.state
}

// WriteLog writes the log to the given writer
func (a *adapter) WriteLog(w io.Writer) {
	a.log.WriteTo(w)
}

// ServeHTTP implements the http.Handler interface
func (a *adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxy, err := a.getProxy(r.Host)
	if err != nil {
		log.Println("[app]", a.Host, "error trying get proxy", err)
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}
	log.Println("[proxy]", zadapter.FullURL(r), "->", proxy.URL)
	proxy.ServeHTTP(w, r)
}

func (a *adapter) start() error {
	a.changeState(zadapter.StatusStarting)
	a.cancelChan = make(chan struct{})

	port, err := findAvailablePort()
	if err != nil {
		e := errors.Context(err, "couldn't find available port")
		a.error(e)
		return e
	}

	a.Port = port

	log.Println("[app] command:", a.ShellCommand)
	if err := a.startApplication(a.ShellCommand); err != nil {
		e := errors.Context(err, "could not start application")
		a.error(e)
		return e
	}

	a.proxies = make(map[string]*rproxy.ReverseProxy)

	go a.tail()
	go a.checkPort()

	return nil
}

func (a *adapter) stop() error {
	a.changeState(zadapter.StatusStopping)
	defer close(a.cancelChan)

	err := a.cmd.Process.Kill()
	if err != nil {
		log.Println("[app]", a.Host, "error trying to stop", err)
		return err
	}

	a.cmd.Wait()

	log.Println("[app]", a.Host, "shutdown and cleaned up")
	a.changeState(zadapter.StatusStopped)
	a.Pid = 0
	return nil
}

func (a *adapter) error(err error) error {
	if a.state == zadapter.StatusStopping || a.state == zadapter.StatusStopped {
		return nil
	}
	a.changeState(zadapter.StatusError)

	log.Println("[app]", a.Host, "ERROR", err)

	if err := a.stop(); err != nil {
		return err
	}

	return nil
}

func (a *adapter) startApplication(command string) error {
	shell := os.Getenv("SHELL")

	command = fmt.Sprintf(command, a.Port, a.Host)
	a.Command = command

	cmd := exec.Command(shell, "-l", "-i", "-c", command)
	cmd.Dir = a.Dir

	cmd.Env = os.Environ()
	if a.EnvPortName != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", a.EnvPortName, a.Port))
	}

	appEnv, err := readEnvFile(cmd.Dir)
	if err != nil {
		log.Println("[app]", a.Host, "ERROR", "couldn't read env file", err)
	}

	for _, pair := range appEnv {
		log.Println("[app]", a.Host, "INFO", "added env var", pair)
		cmd.Env = append(cmd.Env, pair)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	a.stdout = stdout
	cmd.Stderr = cmd.Stdout

	if err = cmd.Start(); err != nil {
		return errors.Context(err, "starting app")
	}

	a.Pid = cmd.Process.Pid
	a.cmd = cmd
	return nil
}

func (a *adapter) tail() {
	c := make(chan error)

	go func() {
		r := bufio.NewReader(a.stdout)

		for {
			line, err := r.ReadString('\n')
			if line != "" {
				a.log.Append(line)
				fmt.Fprintf(os.Stdout, "  [log] %s:%s[%d]: %s", a.Host, a.Port, a.cmd.Process.Pid, line)

				for _, pattern := range a.RestartPatterns {
					if pattern.MatchString(line) {
						a.Stop(errors.New("Restart pattern matched"))
						return
					}
				}
			}

			if err != nil {
				c <- err
				return
			}
		}
	}()

	var err error

	select {
	case err = <-c:
		a.Stop(errors.Context(err, "stdout/stderr closed"))
	}

}

func (a *adapter) checkPort() {
	ticker := time.NewTicker(250 * time.Millisecond)
	timeout := time.After(time.Second * 60)
	defer ticker.Stop()

	for {
		select {
		case <-a.cancelChan:
			log.Println("[app]", a.Host, "cancel channel closed")
			return
		case <-ticker.C:
			c, err := net.Dial("tcp", ":"+a.Port)
			if err == nil {
				defer c.Close()
				log.Println("[app]", a.Host, "port", a.Port, "is available")
				buf := bytes.NewBufferString("")
				a.WriteLog(buf)
				a.BootLog = buf.String()
				a.changeState(zadapter.StatusRunning)
				return
			}
			if err != nil {
				log.Println("[app]", a.Host, "error checking port", a.Port, err)
			}
		case <-timeout:
			log.Println("[app]", a.Host, "timeout waiting for port", a.Port)
			a.error(errors.New("check port timeout"))
			return
		}
	}
}

func (a *adapter) changeState(state zadapter.Status) {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	a.state = state
}

func (a *adapter) getProxy(host string) (*rproxy.ReverseProxy, error) {
	a.proxiesMu.Lock()
	defer a.proxiesMu.Unlock()

	if a.proxies[host] != nil {
		return a.proxies[host], nil
	}

	url, err := url.Parse(a.Scheme + "://127.0.0.1:" + a.Port)
	if err != nil {
		return nil, err
	}
	proxy, err := rproxy.New(url, host)
	if err != nil {
		return nil, err
	}

	a.proxies[host] = proxy

	return proxy, nil
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
