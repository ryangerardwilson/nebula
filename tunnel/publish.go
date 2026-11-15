package tunnel

import (
	"context"
	"fmt"

	"github.com/ryangerardwilson/nebula/tunnel/internal/dns"
	"github.com/ryangerardwilson/nebula/tunnel/internal/routes"
	"github.com/ryangerardwilson/nebula/tunnel/internal/secrets"
	"github.com/ryangerardwilson/nebula/tunnel/internal/xdg"
)

var reservedHost = map[string]bool{
	"ryangerardwilson.com":           true,
	"www.ryangerardwilson.com":       true,
	"ssh.ryangerardwilson.com":       true,
	"notes.ryangerardwilson.com":     true,
	"erza.ryangerardwilson.com":      true,
	"carbide.ryangerardwilson.com":   true,
	"ooolala.ryangerardwilson.com":   true,
	"rohan-tam.ryangerardwilson.com": true,
	"evim.ryangerardwilson.com":      true,
}

// NormalizeHost accepts a label or a name under the zone.
// The apex and names that already belong to something else are refused.
func NormalizeHost(hostname string) (string, error) {
	label, host, err := routes.Normalize(routes.Zone, hostname)
	if err != nil {
		return "", err
	}
	if label == routes.Apex {
		return "", fmt.Errorf("run does not publish the apex")
	}
	if reservedHost[host] {
		return "", fmt.Errorf("hostname %s is reserved", host)
	}
	return host, nil
}

// EnsureDNS creates a proxied CNAME when the hostname has no DNS.
// tunnelID is the host connector, not the laptop connector.
func EnsureDNS(ctx context.Context, hostname, tunnelID string) error {
	tokenFile, err := xdg.PassToken()
	if err != nil {
		return err
	}
	item, err := secrets.Load(tokenFile)
	if err != nil {
		return err
	}
	zone := item.Zone
	if zone == "" {
		zone = routes.Zone
	}
	client := &dns.Client{Token: item.APIToken}
	return client.Ensure(ctx, zone, hostname, tunnelID)
}
