package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"time"

	"github.com/ryangerardwilson/nebula/tunnel/internal/routes"
	"github.com/ryangerardwilson/nebula/tunnel/internal/secrets"
	"github.com/ryangerardwilson/nebula/tunnel/internal/store"
)

const (
	jobScan   = "scan"
	jobAdd    = "add"
	jobRemove = "remove"
)

const (
	scanWait   = 5 * time.Second
	removeWait = 10 * time.Second
	addWait    = 30 * time.Second // store.Add gives DNS 20 s of it
	queueSize  = 32
)

var errBusy = errors.New("busy, try again")

// ops is the I/O the worker does for the screen. The route store owns the
// list; the screen only caches its answers. Tests replace each one.
type ops struct {
	add     func(home, name, port string) (routes.File, error)
	remove  func(home, name string) (routes.File, error)
	service func(ctx context.Context) bool
	listen  func(ctx context.Context, port int) bool
}

// liveOps saves and removes through the route store, and checks systemd
// and the local ports.
func liveOps(load func() (secrets.Tunnel, error), ensure store.Ensure) ops {
	return ops{
		add: func(home, name, port string) (routes.File, error) {
			file, _, err := store.Add(home, name, port, load, ensure)
			return file, err
		},
		remove:  store.Remove,
		service: serviceUp,
		listen:  listening,
	}
}

// job is one piece of screen I/O. The worker stops waiting for run after
// wait and reports label as timed out.
type job struct {
	kind  string
	name  string
	label string
	wait  time.Duration
	run   func(ctx context.Context) result
}

// result is a job's answer. file is the owner's route list after a write.
// late marks the answer of a job that already reported a timeout.
type result struct {
	kind    string
	name    string
	file    routes.File
	service bool
	local   map[int]bool
	err     error
	late    bool
}

// scanJob checks the service and the ports of list as it is now.
func scanJob(o ops, list []routes.Route) job {
	ports := make([]int, 0, len(list))
	for _, route := range list {
		ports = append(ports, route.Port)
	}
	return job{kind: jobScan, label: "scan", wait: scanWait, run: func(ctx context.Context) result {
		r := result{service: o.service(ctx), local: map[int]bool{}}
		for _, port := range ports {
			if _, done := r.local[port]; !done {
				r.local[port] = o.listen(ctx, port)
			}
		}
		if ctx.Err() != nil {
			return result{err: errors.New("scan timed out")}
		}
		return r
	}}
}

// removeJob and saveJob call the store, which takes no context, so the
// worker's wait is their timeout.
func removeJob(o ops, home, name string) job {
	return job{kind: jobRemove, name: name, label: "remove " + name, wait: removeWait, run: func(context.Context) result {
		file, err := o.remove(home, name)
		return result{file: file, err: err}
	}}
}

func saveJob(o ops, home, name, port string) job {
	return job{kind: jobAdd, name: name, label: "save " + name, wait: addWait, run: func(context.Context) result {
		file, err := o.add(home, name, port)
		return result{file: file, err: err}
	}}
}

// worker is the screen's one serial worker. The loop pushes jobs and reads
// results; neither side waits on the other.
type worker struct {
	jobs    chan job
	results chan result
	done    chan struct{}
}

func startWorker() *worker {
	w := &worker{
		jobs:    make(chan job, queueSize),
		results: make(chan result, queueSize),
		done:    make(chan struct{}),
	}
	go w.run()
	return w
}

// push queues j without waiting. It reports false when the queue is full.
func (w *worker) push(j job) bool {
	select {
	case w.jobs <- j:
		return true
	default:
		return false
	}
}

func (w *worker) stop() {
	close(w.done)
}

func (w *worker) run() {
	for {
		select {
		case <-w.done:
			return
		case j := <-w.jobs:
			w.do(j)
		}
	}
}

// do runs one job. Past its wait the screen hears a timeout at once, but
// the job keeps the worker until it ends, so writes stay serial and its own
// answer still reaches the screen.
func (w *worker) do(j job) {
	ctx, cancel := context.WithTimeout(context.Background(), j.wait)
	defer cancel()
	answer := make(chan result, 1)
	go func() { answer <- j.run(ctx) }()
	select {
	case r := <-answer:
		w.send(j, r)
		return
	case <-ctx.Done():
	}
	w.send(j, result{err: fmt.Errorf("%s timed out", j.label)})
	select {
	case r := <-answer:
		r.late = true
		w.send(j, r)
	case <-w.done:
	}
}

func (w *worker) send(j job, r result) {
	r.kind, r.name = j.kind, j.name
	select {
	case w.results <- r:
	case <-w.done:
	}
}

func serviceUp(ctx context.Context) bool {
	return exec.CommandContext(ctx, "systemctl", "--user", "--quiet", "is-active", "tunnel.service").Run() == nil
}

func listening(ctx context.Context, port int) bool {
	dialer := net.Dialer{Timeout: 150 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
