// Package terminalcontrol manages controller terminal availability through a
// private local Unix socket. It exposes no public administration endpoint.
package terminalcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/filelock"
)

type State struct {
	Enabled bool `json:"enabled"`
}
type Status struct {
	Enabled   bool `json:"enabled"`
	Available bool `json:"available"`
	Running   bool `json:"running"`
}
type Control struct {
	dir       string
	lock      *os.File
	state     State
	mu        sync.Mutex
	server    *http.Server
	listener  net.Listener
	done      chan struct{}
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func directory(dataDir string) (string, error) {
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(absolute, 0700); err != nil {
		return "", err
	}
	if err = checkPath(absolute, true, false); err != nil {
		return "", err
	}
	dir := filepath.Join(absolute, "terminal-control")
	if err = os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	if err = checkPath(dir, true, true); err != nil {
		return "", err
	}
	if len(filepath.Join(dir, "control.sock")) > 100 {
		return "", errors.New("terminal control socket path is too long; use a shorter --data-dir")
	}
	return dir, nil
}
func readState(dir string, fallback bool) (State, error) {
	path := filepath.Join(dir, "state.json")
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return State{fallback}, nil
	}
	if err := checkPath(path, false, true); err != nil {
		return State{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return State{}, err
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || info.Size() > 1024 {
		return State{}, errors.New("terminal availability state is too large or unreadable")
	}
	d := json.NewDecoder(io.LimitReader(f, 1024))
	d.DisallowUnknownFields()
	var state struct {
		Enabled *bool `json:"enabled"`
	}
	if d.Decode(&state) != nil || state.Enabled == nil || d.Decode(new(any)) != io.EOF {
		return State{}, errors.New("invalid terminal availability state; inspect state.json")
	}
	return State{*state.Enabled}, nil
}
func writeState(dir string, state State) error {
	target := filepath.Join(dir, "state.json")
	if _, err := os.Lstat(target); err == nil {
		if err = checkPath(target, false, true); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if err = json.NewEncoder(f).Encode(state); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), target); err != nil {
		return err
	}
	parent, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

// Open holds the controller's lifetime lock before reading persisted settings.
func Open(dataDir string, fallback bool) (*Control, error) {
	dir, err := directory(dataDir)
	if err != nil {
		return nil, err
	}
	lock, err := filelock.Acquire(filepath.Join(dir, "owner.lock"))
	if err != nil {
		return nil, fmt.Errorf("terminal controller already running or unavailable: %w", err)
	}
	state, err := readState(dir, fallback)
	if err != nil {
		lock.Close()
		return nil, err
	}
	return &Control{dir: dir, lock: lock, state: state}, nil
}
func (c *Control) Enabled() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.state.Enabled }

// Serve applies live changes only after durable persistence succeeds. apply
// must fence active attachments; available also enforces accounts/HTTPS setup.
func (c *Control) Serve(apply func(bool), available func() bool) error {
	socket := filepath.Join(c.dir, "control.sock")
	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("terminal control path is not a socket")
		}
		if err = checkSocket(socket); err != nil {
			return err
		}
		if err = os.Remove(socket); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	if err = os.Chmod(socket, 0600); err != nil {
		listener.Close()
		return err
	}
	c.listener = listener
	c.done = make(chan struct{})
	c.server = &http.Server{ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			http.Error(w, "controller is stopping", 503)
			return
		}
		if r.URL.Path != "/status" && r.URL.Path != "/start" && r.URL.Path != "/stop" && r.URL.Path != "/restart" {
			http.NotFound(w, r)
			return
		}
		if r.URL.RawQuery != "" || r.ContentLength > 0 || r.ContentLength < 0 {
			http.Error(w, "unexpected request data", 400)
			return
		}
		if r.URL.Path == "/status" {
			if r.Method != "GET" {
				w.WriteHeader(405)
				return
			}
		} else {
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			state := State{r.URL.Path != "/stop"}
			if err := writeState(c.dir, state); err != nil {
				http.Error(w, "could not persist terminal availability", 500)
				return
			}
			if r.URL.Path == "/restart" {
				apply(false)
			}
			if state.Enabled != c.state.Enabled || r.URL.Path == "/restart" {
				apply(state.Enabled)
			}
			c.state = state
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(Status{c.state.Enabled, available(), true})
	})}
	go func() { defer close(c.done); _ = c.server.Serve(listener) }()
	return nil
}
func (c *Control) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		if c.server != nil {
			_ = c.server.Close()
			<-c.done
		}
		if c.listener != nil {
			_ = os.Remove(filepath.Join(c.dir, "control.sock"))
		}
		c.closeErr = c.lock.Close()
	})
	return c.closeErr
}

func Command(ctx context.Context, dataDir, action string, fallback bool) (Status, error) {
	if action != "start" && action != "stop" && action != "restart" && action != "status" {
		return Status{}, errors.New("use reet terminal start|stop|restart|status")
	}
	dir, err := directory(dataDir)
	if err != nil {
		return Status{}, err
	}
	lock, err := filelock.Acquire(filepath.Join(dir, "owner.lock"))
	if err == nil {
		defer lock.Close()
		state, err := readState(dir, fallback)
		if err != nil {
			return Status{}, err
		}
		if action == "restart" {
			return Status{}, errors.New("controller is not running; start reet serve first")
		}
		if action != "status" {
			state.Enabled = action == "start"
			if err = writeState(dir, state); err != nil {
				return Status{}, err
			}
		}
		return Status{Enabled: state.Enabled}, nil
	}
	if !errors.Is(err, filelock.ErrLocked) {
		return Status{}, err
	}
	socket := filepath.Join(dir, "control.sock")
	if err = checkSocket(socket); err != nil {
		return Status{}, fmt.Errorf("controller is starting or its local control is unavailable: %w", err)
	}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	method := "POST"
	if action == "status" {
		method = "GET"
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://localhost/"+action, nil)
	if err != nil {
		return Status{}, err
	}
	response, err := client.Do(req)
	if err != nil {
		return Status{}, fmt.Errorf("controller local control unavailable; no change confirmed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Status{}, fmt.Errorf("controller rejected terminal command (HTTP %d); no change confirmed", response.StatusCode)
	}
	var status Status
	if err = json.NewDecoder(io.LimitReader(response.Body, 2048)).Decode(&status); err != nil {
		return Status{}, err
	}
	return status, nil
}
