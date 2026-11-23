package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ryangerardwilson/nebula/tunnel/internal/routes"
	"github.com/ryangerardwilson/nebula/tunnel/internal/secrets"
)

type Ensure func(ctx context.Context, zone, hostname, tunnelID string) error

// Add publishes DNS first, then saves the route. A refused name is not saved.
func Add(home, name, portText string, load func() (secrets.Tunnel, error), ensure Ensure) (routes.File, string, error) {
	port, err := routes.ParsePort(portText)
	if err != nil {
		return routes.File{}, "", err
	}
	file, err := routes.EnsureFile(routes.Path(home))
	if err != nil {
		return routes.File{}, "", err
	}
	next, host, err := routes.Upsert(file, name, port)
	if err != nil {
		return routes.File{}, "", err
	}
	if err := publish(next.Zone, host, load, ensure); err != nil {
		return routes.File{}, "", err
	}
	// DNS ran without the lock; the list is read again under it, so an add
	// or remove that finished meanwhile is kept.
	var saved routes.File
	err = withRoutes(home, func(file routes.File) (routes.File, error) {
		saved, _, err = routes.Upsert(file, name, port)
		return saved, err
	})
	if err != nil {
		return routes.File{}, "", err
	}
	return saved, host, nil
}

// withRoutes reads the route list, applies change, and saves the result
// while holding the route lock, so two writers never lose each other's
// change. It holds the lock for local work only.
func withRoutes(home string, change func(routes.File) (routes.File, error)) error {
	path := routes.Path(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	file, err := routes.EnsureFile(path)
	if err != nil {
		return err
	}
	next, err := change(file)
	if err != nil {
		return err
	}
	return routes.Save(path, next)
}

// publish makes sure the hostname points at the tunnel. With no ensure,
// DNS is left alone.
func publish(zone, host string, load func() (secrets.Tunnel, error), ensure Ensure) error {
	if ensure == nil {
		return nil
	}
	if load == nil {
		return fmt.Errorf("missing credentials")
	}
	tunnel, err := load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if zone == "" {
		zone = tunnel.Zone
	}
	return ensure(ctx, zone, host, tunnel.ID)
}

func Remove(home, name string) (routes.File, error) {
	var next routes.File
	err := withRoutes(home, func(file routes.File) (routes.File, error) {
		var err error
		next, err = routes.Remove(file, name)
		return next, err
	})
	if err != nil {
		return routes.File{}, err
	}
	return next, nil
}
