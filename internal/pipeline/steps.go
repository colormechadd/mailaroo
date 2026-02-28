package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/colormechadd/maileroo/pkg/models"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/google/uuid"
	"github.com/zaccone/spf"
)

// ValidateSPF performs SPF check and returns status, details, and error
func ValidateSPF(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	res, explanation, err := spf.CheckHost(ictx.RemoteIP, ictx.FromAddress, ictx.FromAddress)
	status := StatusFail
	if res == spf.Pass {
		status = StatusPass
	} else if res == spf.Neutral || res == spf.None {
		status = StatusNeutral
	}
	return status, map[string]any{"result": res.String(), "explanation": explanation}, err
}

// ValidateDKIM performs DKIM check and returns status, details, and error
func ValidateDKIM(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	r := bytes.NewReader(ictx.RawMessage)
	verifications, err := dkim.Verify(r)
	if err != nil {
		return StatusError, nil, err
	}

	status := StatusNone
	results := []any{}
	for _, v := range verifications {
		vErr := v.Err
		vStatus := StatusPass
		if vErr != nil {
			vStatus = StatusFail
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

// ValidateRBL checks the remote IP against configured RBL servers
func ValidateRBL(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	if len(p.cfg.Spam.RBLServers) == 0 {
		return StatusSkipped, nil, nil
	}

	ip := ictx.RemoteIP
	if ip == nil {
		return StatusSkipped, nil, nil
	}

	// Reverse IP for DNS lookup (e.g. 1.2.3.4 -> 4.3.2.1)
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
	// TODO: Support IPv6 reversal if needed
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

// Deliver handles both storage and database persistence in one logical step
func Deliver(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	// 1. Store raw email
	ictx.StorageKey = fmt.Sprintf("%s/%s.eml", ictx.TargetMailboxID, ictx.ID)
	if err := p.storage.Save(ctx, ictx.StorageKey, bytes.NewReader(ictx.RawMessage)); err != nil {
		return StatusError, nil, fmt.Errorf("storage save failed: %w", err)
	}

	// 2. Parse and Persist Metadata
	mr, err := mail.CreateReader(bytes.NewReader(ictx.RawMessage))
	if err != nil {
		return StatusError, nil, fmt.Errorf("failed to create mail reader: %w", err)
	}
	defer mr.Close()

	subject, _ := mr.Header.Subject()
	msgID, _ := mr.Header.MessageID()
	if msgID == "" {
		msgID = ictx.ID.String()
	}

	inReplyTo, _ := mr.Header.Text("In-Reply-To")
	referencesRaw, _ := mr.Header.Text("References")
	references := parseReferences(referencesRaw)

	// Threading logic
	lookups := []string{}
	if inReplyTo != "" {
		lookups = append(lookups, strings.Trim(inReplyTo, "<> "))
	}
	for _, r := range references {
		lookups = append(lookups, strings.Trim(r, "<> "))
	}

	var threadID uuid.UUID
	if len(lookups) > 0 {
		threadID, _ = p.db.FindThreadIDByMessageIDs(ctx, ictx.TargetMailboxID, lookups)
	}

	if threadID == uuid.Nil {
		threadID = uuid.New()
		newThread := &models.Thread{
			ID:        threadID,
			MailboxID: ictx.TargetMailboxID,
			Subject:   subject,
		}
		if err := p.db.CreateThread(ctx, newThread); err != nil {
			return StatusError, nil, fmt.Errorf("thread creation failed: %w", err)
		}
	}

	emailID := uuid.New()
	email := &models.Email{
		ID:               emailID,
		MailboxID:        ictx.TargetMailboxID,
		ThreadID:         &threadID,
		AddressMappingID: &ictx.AddressMappingID,
		IngestionID:      &ictx.ID,
		MessageID:        msgID,
		InReplyTo:        &inReplyTo,
		References:       &referencesRaw,
		Subject:          subject,
		FromAddress:      ictx.FromAddress,
		ToAddress:        ictx.ToAddresses[0],
		StorageKey:       ictx.StorageKey,
		Size:             int64(len(ictx.RawMessage)),
		ReceiveDatetime:  time.Now(),
		IsRead:           false,
		IsStar:           false,
	}

	if err := p.db.CreateEmail(ctx, email); err != nil {
		return StatusError, nil, fmt.Errorf("email creation failed: %w", err)
	}

	// 3. Process attachments
	attachmentCount := 0
	for {
		pPart, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Error("error reading message part", "ingestion_id", ictx.ID, "error", err)
			break
		}

		switch h := pPart.Header.(type) {
		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			contentType, _, _ := h.ContentType()
			attID := uuid.New()
			attKey := fmt.Sprintf("%s/attachments/%s/%s_%s", ictx.TargetMailboxID, emailID, attID, filename)

			var buf bytes.Buffer
			size, err := io.Copy(&buf, pPart.Body)
			if err != nil {
				slog.Error("failed to read attachment body", "ingestion_id", ictx.ID, "filename", filename, "error", err)
				continue
			}

			if err := p.storage.Save(ctx, attKey, bytes.NewReader(buf.Bytes())); err != nil {
				slog.Error("failed to save attachment to storage", "ingestion_id", ictx.ID, "filename", filename, "error", err)
				continue
			}

			att := &models.EmailAttachment{
				ID:          attID,
				EmailID:     emailID,
				Filename:    filename,
				ContentType: contentType,
				Size:        size,
				StorageKey:  attKey,
			}

			if err := p.db.CreateAttachment(ctx, att); err != nil {
				slog.Error("failed to save attachment metadata", "ingestion_id", ictx.ID, "filename", filename, "error", err)
				continue
			}
			attachmentCount++
		}
	}

	return StatusPass, map[string]any{
		"email_id":    email.ID,
		"thread_id":   threadID,
		"attachments": attachmentCount,
		"storage_key": ictx.StorageKey,
	}, nil
}

func parseReferences(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Fields(raw)
}
