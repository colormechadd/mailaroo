package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/colormechadd/maileroo/internal/mail"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-msgauth/dmarc"
	"github.com/zaccone/spf"
)

// ValidateSender performs SPF, DKIM and optionally DMARC checks.
// It requires either SPF or DKIM to pass.
func ValidateSender(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	domain := extractDomain(ictx.FromAddress)

	// 1. SPF Check
	spfRes, spfExp, spfErr := spf.CheckHost(ictx.RemoteIP, domain, ictx.FromAddress)
	spfPass := (spfRes == spf.Pass)

	// 2. DKIM Check
	dkimStatus, dkimResults, _ := checkDKIM(ictx.RawMessage)
	dkimPass := (dkimStatus == StatusPass)

	results := map[string]any{
		"spf": map[string]any{
			"result":      spfRes.String(),
			"explanation": spfExp,
			"error":       spfErr,
		},
		"dkim": dkimResults,
	}

	if spfPass || dkimPass {
		return StatusPass, results, nil
	}

	// 3. DMARC Check (only if both failed)
	dmarcRecord, dmarcErr := dmarc.Lookup(domain)
	if dmarcErr == nil && dmarcRecord != nil {
		results["dmarc"] = map[string]any{
			"policy": string(dmarcRecord.Policy),
			"status": "found",
		}

		if dmarcRecord.Policy == dmarc.PolicyNone {
			return StatusPass, results, nil
		}

		if dmarcRecord.Policy == dmarc.PolicyReject || dmarcRecord.Policy == dmarc.PolicyQuarantine {
			return StatusFail, results, nil
		}
	} else {
		results["dmarc"] = map[string]any{
			"status": "not_found",
			"error":  dmarcErr,
		}
	}

	return StatusFail, results, nil
}

func checkDKIM(raw []byte) (StepStatus, []any, error) {
	r := bytes.NewReader(raw)
	verifications, err := dkim.Verify(r)
	if err != nil {
		return StatusError, nil, err
	}

	status := StatusNone
	results := []any{}
	for _, v := range verifications {
		vErr := v.Err
		vStatus := "pass"
		if vErr != nil {
			vStatus = "fail"
			status = StatusFail
		} else if status != StatusFail {
			status = StatusPass
		}
		results = append(results, map[string]any{
			"domain": v.Domain,
			"status": vStatus,
			"error":  vErr,
		})
	}
	return status, results, nil
}

func extractDomain(address string) string {
	parts := strings.Split(address, "@")
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

// ValidateRBL checks the remote IP against configured RBL servers
func ValidateRBL(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	if len(p.cfg.Spam.RBLServers) == 0 {
		return StatusSkipped, nil, nil
	}

	ip := ictx.RemoteIP
	if ip == nil {
		return StatusSkipped, nil, nil
	}

	reversedIP := reverseIP(ip)
	if reversedIP == "" {
		return StatusSkipped, nil, nil
	}

	hits := []string{}
	for _, server := range p.cfg.Spam.RBLServers {
		lookup := fmt.Sprintf("%s.%s", reversedIP, server)
		ips, err := net.LookupIP(lookup)
		if err == nil && len(ips) > 0 {
			hits = append(hits, server)
		}
	}

	if len(hits) > 0 {
		return StatusFail, map[string]any{"rbl_hits": hits}, nil
	}

	return StatusPass, map[string]any{"rbl_hits": hits}, nil
}

func reverseIP(ip net.IP) string {
	if ipv4 := ip.To4(); ipv4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d", ipv4[3], ipv4[2], ipv4[1], ipv4[0])
	}
	return ""
}

// CheckBlockingRules checks if the from address is blocked for the target mailbox
func CheckBlockingRules(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	blocked, err := p.db.IsBlockedByMailboxRules(ctx, ictx.TargetMailboxID, ictx.FromAddress)
	if err != nil {
		return StatusError, nil, err
	}

	if blocked {
		return StatusFail, map[string]any{"blocked": true}, nil
	}

	return StatusPass, map[string]any{"blocked": false}, nil
}

// Notify broadcasts a new-mail event to the hub
func Notify(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	p.hub.Broadcast(Event{
		UserID:    ictx.UserID,
		MailboxID: ictx.TargetMailboxID,
		Type:      "new-mail",
	})

	return StatusPass, nil, nil
}

// Deliver handles both storage and database persistence in one logical step
func Deliver(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	email, err := p.mail.Persist(ctx, mail.PersistOptions{
		MailboxID:        ictx.TargetMailboxID,
		RawMessage:       ictx.RawMessage,
		IsOutbound:       false,
		UserID:           ictx.UserID,
		IngestionID:      &ictx.ID,
		AddressMappingID: &ictx.AddressMappingID,
	})
	if err != nil {
		return StatusError, nil, err
	}

	ictx.StorageKey = email.StorageKey

	return StatusPass, map[string]any{
		"email_id":    email.ID,
		"thread_id":   email.ThreadID,
		"storage_key": email.StorageKey,
	}, nil
}
