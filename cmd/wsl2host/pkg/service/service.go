package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shayne/go-wsl2-host/internal/wsl2hosts"

	"github.com/shayne/go-wsl2-host/pkg/hostsapi"

	"github.com/shayne/go-wsl2-host/pkg/wslapi"
)

const tld = ".wsl"

var hostnamereg, _ = regexp.Compile("[^A-Za-z0-9]+")

func distroNameToHostname(distroname string) string {
	// Ubuntu-18.04
	// => ubuntu1804.wsl
	hostname := strings.ToLower(distroname)
	hostname = hostnamereg.ReplaceAllString(hostname, "")
	return hostname + tld
}

// Run main entry point to service logic
func Run() error {
	infos, err := wslapi.GetAllInfo()
	if err != nil {
		return fmt.Errorf("failed to get infos: %w", err)
	}

	hapi, err := hostsapi.CreateAPI("wsl2-host") // filtere only managed host entries
	if err != nil {
		return fmt.Errorf("failed to create hosts api: %w", err)
	}

	updated := false
	hostentries := hapi.Entries()

	for _, i := range infos {
		hostname := distroNameToHostname(i.Name)
		// remove stopped distros
		if i.Running == false {
			err := hapi.RemoveEntry(hostname)
			if err == nil {
				updated = true
			}
			continue
		}

		// update IPs of running distros
		ip, err := wslapi.GetIP(i.Name)
		if he, exists := hostentries[hostname]; exists {
			if err != nil {
				return fmt.Errorf("failed to get IP for distro %q: %w", i.Name, err)
			}
			if he.IP != ip {
				updated = true
				he.IP = ip
			}
		} else {
			// add running distros not present
			err := hapi.AddEntry(&hostsapi.HostEntry{
				Hostname: hostname,
				IP:       ip,
				Comment:  wsl2hosts.DefaultComment(),
			})
			if err == nil {
				updated = true
			}
		}
	}

	// process aliases
	defdistro, _ := wslapi.GetDefaultDistro()
	if err != nil {
		return fmt.Errorf("GetDefaultDistro failed: %w", err)
	}
	var aliasmap = make(map[string]interface{})
	defdistroip, _ := wslapi.GetIP(defdistro.Name)
	if defdistro.Running {
		aliases, err := wslapi.GetHostAliases()
		if err == nil {
			for _, a := range aliases {
				aliasmap[a] = nil
			}
		}
	}
	// update entries after distro processing
	hostentries = hapi.Entries()
	for _, he := range hostentries {
		if !wsl2hosts.IsAlias(he.Comment) {
			continue
		}
		// update IP for aliases when running and if it exists in aliasmap
		if _, ok := aliasmap[he.Hostname]; ok && defdistro.Running {
			if he.IP != defdistroip {
				updated = true
				he.IP = defdistroip
			}
		} else { // remove entry when not running or not in aliasmap
			err := hapi.RemoveEntry(he.Hostname)
			if err == nil {
				updated = true
			}
		}
	}

	for hostname := range aliasmap {
		// add new aliases
		if _, ok := hostentries[hostname]; !ok && defdistro.Running {
			err := hapi.AddEntry(&hostsapi.HostEntry{
				IP:       defdistroip,
				Hostname: hostname,
				Comment:  wsl2hosts.DistroComment(defdistro.Name),
			})
			if err == nil {
				updated = true
			}
		}
	}

	if updated {
		err = hapi.Write()
		if err != nil {
			return fmt.Errorf("failed to write hosts file: %w", err)
		}
	}

	return nil
}