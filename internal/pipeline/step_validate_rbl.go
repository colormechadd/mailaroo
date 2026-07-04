package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"net"
)

// ValidateRBL checks the remote IP against configured RBL servers
func ValidateRBL(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	if len(p.cfg.Spam.RBLServers) == 0 {
		slog.Debug("rbl: no RBL servers configured, skipping", "ingestion_id", ictx.ID)
		return StatusSkipped, nil, nil
	}

	ip := ictx.RemoteIP
	if ip == nil {
		slog.Debug("rbl: no remote IP, skipping", "ingestion_id", ictx.ID)
		return StatusSkipped, nil, nil
	}

	reversedIP := reverseIP(ip)
	if reversedIP == "" {
		slog.Debug("rbl: could not reverse IP, skipping", "ingestion_id", ictx.ID, "remote_ip", ip.String())
		return StatusSkipped, nil, nil
	}

	slog.Info("rbl: checking IP", "ingestion_id", ictx.ID, "remote_ip", ip.String(), "reversed_ip", reversedIP)

	hits := []string{}
	for _, server := range p.cfg.Spam.RBLServers {
		lookup := fmt.Sprintf("%s.%s", reversedIP, server)
		ips, err := net.LookupIP(lookup)
		if err == nil && len(ips) > 0 {
			slog.Warn("rbl: hit", "ingestion_id", ictx.ID, "remote_ip", ip.String(), "server", server, "lookup", lookup, "resolved_to", ips)
			hits = append(hits, server)
		} else {
			slog.Debug("rbl: not listed", "ingestion_id", ictx.ID, "remote_ip", ip.String(), "server", server, "lookup", lookup, "lookup_error", err)
		}
	}

	if len(hits) > 0 {
		slog.Warn("rbl: IP flagged as spam", "ingestion_id", ictx.ID, "remote_ip", ip.String(), "hits", hits)
		return StatusFail, map[string]any{"rbl_hits": hits}, nil
	}

	slog.Info("rbl: IP passed all checks", "ingestion_id", ictx.ID, "remote_ip", ip.String())
	return StatusPass, map[string]any{"rbl_hits": hits}, nil
}

func reverseIP(ip net.IP) string {
	if ipv4 := ip.To4(); ipv4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d", ipv4[3], ipv4[2], ipv4[1], ipv4[0])
	}
	return ""
}
