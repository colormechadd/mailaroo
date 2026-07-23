package pipeline

import (
	"context"
	"fmt"
	"net"
)

var (
	rblListedMin  = net.ParseIP("127.0.0.2")
	rblListedMax  = net.ParseIP("127.0.0.11")

	rblErrorRange = net.IPNet{
		IP:   net.ParseIP("127.255.255.0"),
		Mask: net.CIDRMask(24, 32),
	}
)

func isRBLListed(ips []net.IP) bool {
	for _, ip := range ips {
		ipv4 := ip.To4()
		if ipv4 == nil {
			continue
		}
		if rblErrorRange.Contains(ip) {
			continue
		}
		ipNum := uint32(ipv4[0])<<24 | uint32(ipv4[1])<<16 | uint32(ipv4[2])<<8 | uint32(ipv4[3])
		minNum := uint32(rblListedMin.To4()[0])<<24 | uint32(rblListedMin.To4()[1])<<16 | uint32(rblListedMin.To4()[2])<<8 | uint32(rblListedMin.To4()[3])
		maxNum := uint32(rblListedMax.To4()[0])<<24 | uint32(rblListedMax.To4()[1])<<16 | uint32(rblListedMax.To4()[2])<<8 | uint32(rblListedMax.To4()[3])
		if ipNum >= minNum && ipNum <= maxNum {
			return true
		}
	}
	return false
}

// ValidateRBL checks the remote IP against configured RBL servers
func ValidateRBL(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	if len(p.cfg.Spam.RBLServers) == 0 {
		p.logger.Debug("rbl: no RBL servers configured, skipping", "ingestion_id", ictx.ID)
		return StatusSkipped, nil, nil
	}

	ip := ictx.RemoteIP
	if ip == nil {
		p.logger.Debug("rbl: no remote IP, skipping", "ingestion_id", ictx.ID)
		return StatusSkipped, nil, nil
	}

	reversedIP := reverseIP(ip)
	if reversedIP == "" {
		p.logger.Debug("rbl: could not reverse IP, skipping", "ingestion_id", ictx.ID, "remote_ip", ip.String())
		return StatusSkipped, nil, nil
	}

	p.logger.Info("rbl: checking IP", "ingestion_id", ictx.ID, "remote_ip", ip.String(), "reversed_ip", reversedIP)

	hits := []string{}
	for _, server := range p.cfg.Spam.RBLServers {
		lookup := fmt.Sprintf("%s.%s", reversedIP, server)
		ips, err := net.LookupIP(lookup)
		if err == nil && isRBLListed(ips) {
			p.logger.Warn("rbl: hit", "ingestion_id", ictx.ID, "remote_ip", ip.String(), "server", server, "lookup", lookup, "resolved_to", ips)
			hits = append(hits, server)
		} else {
			p.logger.Debug("rbl: not listed", "ingestion_id", ictx.ID, "remote_ip", ip.String(), "server", server, "lookup", lookup, "lookup_error", err)
		}
	}

	if len(hits) > 0 {
		p.logger.Warn("rbl: IP flagged as spam", "ingestion_id", ictx.ID, "remote_ip", ip.String(), "hits", hits)
		return StatusFail, map[string]any{"rbl_hits": hits}, nil
	}

	p.logger.Info("rbl: IP passed all checks", "ingestion_id", ictx.ID, "remote_ip", ip.String())
	return StatusPass, map[string]any{"rbl_hits": hits}, nil
}

func reverseIP(ip net.IP) string {
	if ip == nil {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d", ipv4[3], ipv4[2], ipv4[1], ipv4[0])
	}
	ipv6 := ip.To16()
	if ipv6 == nil {
		return ""
	}
	// Nibble format for RBL: expand to 32 hex digits, reverse, dot-separate
	const hex = "0123456789abcdef"
	b := make([]byte, 0, 63)
	for i := 15; i >= 0; i-- {
		if i < 15 {
			b = append(b, '.')
		}
		b = append(b, hex[ipv6[i]&0x0f])
		b = append(b, '.')
		b = append(b, hex[ipv6[i]>>4])
	}
	return string(b)
}
