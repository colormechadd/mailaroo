package pipeline

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

func CheckRateLimit(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error) {
	if ictx.RemoteIP == nil || ictx.RemoteIP.IsLoopback() {
		return StatusSkipped, nil, nil
	}

	rc := p.cfg.RateLimit
	if rc.SMTPConnectionsPerMinute <= 0 {
		return StatusSkipped, nil, nil
	}

	ip := ictx.RemoteIP.String()

	p.mu.Lock()
	p.limiterLastSeen[ip] = time.Now()

	p.cleanupTick++
	if p.cleanupTick >= limiterCleanupInterval {
		p.cleanupTick = 0
		threshold := time.Now().Add(-limiterStaleAge)
		for k, last := range p.limiterLastSeen {
			if last.Before(threshold) {
				delete(p.limiters, k)
				delete(p.limiterLastSeen, k)
				delete(p.violations, k)
			}
		}
	}

	limiter, ok := p.limiters[ip]
	if !ok {
		r := rate.Every(time.Minute / time.Duration(rc.SMTPConnectionsPerMinute))
		limiter = rate.NewLimiter(r, rc.SMTPConnectionsPerMinute)
		p.limiters[ip] = limiter
	}
	p.mu.Unlock()

	if !limiter.Allow() {
		p.mu.Lock()
		p.violations[ip]++
		violations := p.violations[ip]
		p.mu.Unlock()

		p.logger.Warn("rate limit exceeded", "ip", ip, "violations", violations)

		if rc.SMTPAutoBlockThreshold > 0 && violations >= rc.SMTPAutoBlockThreshold {
			until := time.Now().Add(rc.SMTPAutoBlockDuration)
			if err := p.db.AddIPBlock(ctx, ictx.RemoteIP, "auto-blocked: rate limit exceeded", &until); err != nil {
				p.logger.Error("failed to auto-block ip", "ip", ip, "error", err)
			} else {
				p.logger.Warn("ip auto-blocked", "ip", ip, "until", until)
				p.mu.Lock()
				delete(p.violations, ip)
				p.mu.Unlock()
			}
		}

		return StatusFail, nil, &RejectError{
			Code:    421,
			Message: "Too many connections from your IP, please try again later",
		}
	}

	return StatusPass, nil, nil
}
