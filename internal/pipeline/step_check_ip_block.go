package pipeline

import (
	"context"
	"fmt"
)

func CheckIPBlock(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	if ictx.RemoteIP == nil || ictx.RemoteIP.IsLoopback() {
		return StatusSkipped, nil, nil
	}

	blocked, err := p.db.IsIPBlocked(ctx, ictx.RemoteIP)
	if err != nil {
		return StatusError, nil, fmt.Errorf("ip block check failed: %w", err)
	}
	if blocked {
		return StatusFail, map[string]any{"blocked": true, "ip": ictx.RemoteIP.String()}, nil
	}

	return StatusPass, map[string]any{"blocked": false}, nil
}
