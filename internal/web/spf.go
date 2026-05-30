package web

import (
	"context"
	"net"
	"strings"
)

// spfAuthorizes checks whether spfRecord authorizes any of the given IPs,
// following include: chains up to 3 levels deep.
func spfAuthorizes(ctx context.Context, r *net.Resolver, domain, spfRecord string, serverIPs []string, depth int) bool {
	if depth > 3 {
		return false
	}
	for _, tok := range strings.Fields(spfRecord) {
		tok = strings.TrimLeft(tok, "+-~?")
		switch {
		case strings.HasPrefix(tok, "ip4:"), strings.HasPrefix(tok, "ip6:"):
			cidr := tok[4:]
			if spfIPMatches(cidr, serverIPs) {
				return true
			}
		case strings.HasPrefix(tok, "include:"):
			inc := strings.TrimPrefix(tok, "include:")
			incTXTs, err := r.LookupTXT(ctx, inc)
			if err != nil {
				continue
			}
			for _, t := range incTXTs {
				if strings.HasPrefix(t, "v=spf1") && spfAuthorizes(ctx, r, inc, t, serverIPs, depth+1) {
					return true
				}
			}
		case tok == "mx":
			mxs, err := r.LookupMX(ctx, domain)
			if err != nil {
				continue
			}
			for _, mx := range mxs {
				hosts, err := r.LookupHost(ctx, mx.Host)
				if err != nil {
					continue
				}
				if spfIPMatches("", hosts) {
					return true
				}
			}
		case tok == "a":
			hosts, err := r.LookupHost(ctx, domain)
			if err != nil {
				continue
			}
			for _, sip := range serverIPs {
				for _, h := range hosts {
					if h == sip {
						return true
					}
				}
			}
		}
	}
	return false
}

func spfIPMatches(cidr string, serverIPs []string) bool {
	if cidr == "" {
		return false
	}
	var network *net.IPNet
	var single net.IP
	if strings.Contains(cidr, "/") {
		var err error
		_, network, err = net.ParseCIDR(cidr)
		if err != nil {
			return false
		}
	} else {
		single = net.ParseIP(cidr)
		if single == nil {
			return false
		}
	}
	for _, s := range serverIPs {
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		if network != nil && network.Contains(ip) {
			return true
		}
		if single != nil && single.Equal(ip) {
			return true
		}
	}
	return false
}
