package pipeline

import (
	"context"

	"github.com/colormechadd/mailaroo/internal/classifier"
)

func ClassifyEmail(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	if p.classifier == nil {
		return StatusSkipped, nil, nil
	}

	msg := parsedMessageFromRaw(ictx.FromAddress, ictx.ToAddresses, ictx.RawMessage)

	input := classifier.ClassifyInput{
		From:    msg.From,
		To:      msg.To,
		Subject: msg.Subject,
		Body:    msg.Body,
	}

	category, err := p.classifier.Classify(ctx, input)
	if err != nil {
		p.logger.Warn("classifier error, skipping", "error", err)
		return StatusSkipped, nil, nil
	}

	if err := p.db.SetEmailCategory(ctx, ictx.EmailID, category); err != nil {
		return StatusError, nil, err
	}

	ictx.Category = string(category)

	return StatusPass, map[string]any{
		"category": category,
	}, nil
}
