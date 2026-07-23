package pipeline

import (
	"bytes"
	"context"
	"net/mail"
	"strings"

	"github.com/colormechadd/mailaroo/pkg/models"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-msgauth/dmarc"
	"github.com/zaccone/spf"
)

type dkimResult struct {
	Domain string
	Pass   bool
	Err    error
}

// ValidateSender performs SPF, DKIM and DMARC checks with alignment
// per RFC 7489. For DMARC to pass, either SPF or DKIM must authenticate
// a domain that aligns with the RFC5322.From header domain.
func ValidateSender(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	envelopeDomain := extractDomain(ictx.FromAddress)
	headerFromDomain := extractHeaderFromDomain(ictx.RawMessage)

	// 1. SPF Check
	spfRes, spfExp, spfErr := spf.CheckHost(ictx.RemoteIP, envelopeDomain, ictx.FromAddress)
	spfPass := (spfRes == spf.Pass)

	// 2. DKIM Check
	dkimStatus, dkimResults, _ := checkDKIM(ictx.RawMessage)
	dkimPass := (dkimStatus == StatusPass)

	// Build detail maps
	spfDetail := map[string]any{
		"result":      spfRes.String(),
		"explanation": spfExp,
		"error":       spfErr,
	}
	dkimDetail := make([]any, 0, len(dkimResults))
	for _, r := range dkimResults {
		vStatus := "pass"
		if !r.Pass {
			vStatus = "fail"
		}
		dkimDetail = append(dkimDetail, map[string]any{
			"domain": r.Domain,
			"status": vStatus,
			"error":  r.Err,
		})
	}

	results := map[string]any{
		"spf":  spfDetail,
		"dkim": dkimDetail,
	}

	// 3. DMARC Check
	dmarcDomain := headerFromDomain
	if dmarcDomain == "" {
		dmarcDomain = envelopeDomain
	}
	dmarcRecord, dmarcErr := dmarc.Lookup(dmarcDomain)
	if dmarcErr == nil && dmarcRecord != nil {
		results["dmarc"] = map[string]any{
			"policy": string(dmarcRecord.Policy),
			"status": "found",
		}

		// DMARC alignment: authenticated domain must match the RFC5322.From domain
		spfAligned := spfPass && envelopeDomain != "" && headerFromDomain != "" &&
			strings.EqualFold(envelopeDomain, headerFromDomain)
		dkimAligned := false
		for _, r := range dkimResults {
			if r.Pass && strings.EqualFold(r.Domain, headerFromDomain) {
				dkimAligned = true
				break
			}
		}

		if spfAligned || dkimAligned {
			return StatusPass, results, nil
		}

		// Neither SPF nor DKIM aligned — apply DMARC policy
		switch dmarcRecord.Policy {
		case dmarc.PolicyNone:
			return StatusPass, results, nil
		case dmarc.PolicyReject, dmarc.PolicyQuarantine:
			ictx.FilterAction = models.FilterActionQuarantine
			return StatusQuarantined, results, nil
		default:
			ictx.FilterAction = models.FilterActionQuarantine
			return StatusQuarantined, results, nil
		}
	}

	results["dmarc"] = map[string]any{
		"status": "not_found",
		"error":  dmarcErr,
	}

	// No DMARC policy — fall back to traditional SPF/DKIM validation
	if spfPass || dkimPass {
		return StatusPass, results, nil
	}

	// Both SPF and DKIM failed with no DMARC policy — conservative quarantine
	ictx.FilterAction = models.FilterActionQuarantine
	return StatusQuarantined, results, nil
}

func checkDKIM(raw []byte) (StepStatus, []dkimResult, error) {
	r := bytes.NewReader(raw)
	verifications, err := dkim.Verify(r)
	if err != nil {
		return StatusError, nil, err
	}

	status := StatusNone
	results := []dkimResult{}
	for _, v := range verifications {
		pass := v.Err == nil
		if pass {
			status = StatusPass
		} else if status != StatusPass {
			status = StatusFail
		}
		results = append(results, dkimResult{
			Domain: v.Domain,
			Pass:   pass,
			Err:    v.Err,
		})
	}
	return status, results, nil
}

func extractDomain(address string) string {
	if address == "" {
		return ""
	}
	parts := strings.Split(address, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[1]))
}

// extractHeaderFromDomain parses the RFC5322.From header from the raw message
// and returns the domain portion in lower case.
func extractHeaderFromDomain(raw []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	from := msg.Header.Get("From")
	if from == "" {
		return ""
	}
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return ""
	}
	parts := strings.Split(addr.Address, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[1]))
}
