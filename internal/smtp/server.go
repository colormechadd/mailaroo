package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/colormechadd/mailaroo/internal/config"
	"github.com/colormechadd/mailaroo/internal/db"
	"github.com/colormechadd/mailaroo/internal/pipeline"
	"github.com/colormechadd/mailaroo/pkg/auth"
	"github.com/colormechadd/mailaroo/pkg/models"
	gosmtp "github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// RecipientInfo stores mapping details for a recipient
type RecipientInfo struct {
	Address   string
	MailboxID uuid.UUID
	MappingID uuid.UUID
}

// SMTPAuthDB provides user authentication and sending permission checks.
type SMTPAuthDB interface {
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
	IsAuthorizedSendingAddress(ctx context.Context, userID uuid.UUID, address string) (bool, error)
}

// SMTPOutboundDB provides outbound job creation.
type SMTPOutboundDB interface {
	InsertOutboundJob(ctx context.Context, emailID *uuid.UUID, fromAddress string, recipients []string, rawMessage []byte) (*models.OutboundJob, error)
}

// Backend implements smtp.Backend
type Backend struct {
	cfg         config.SMTPConfig
	rateCfg     config.RateLimitConfig
	db          db.MailDB
	rateLimitDB db.RateLimitDB
	pipeline    *pipeline.Pipeline
	authDB      SMTPAuthDB
	outboundDB  SMTPOutboundDB
	logger      *slog.Logger
	mu          sync.Mutex
	limiters    map[string]*rate.Limiter
	violations  map[string]int
	userLimit   map[uuid.UUID]*rate.Limiter
}

func (bkd *Backend) getLimiter(ip string) *rate.Limiter {
	bkd.mu.Lock()
	defer bkd.mu.Unlock()
	if l, ok := bkd.limiters[ip]; ok {
		return l
	}
	r := rate.Every(time.Minute / time.Duration(bkd.rateCfg.SMTPConnectionsPerMinute))
	l := rate.NewLimiter(r, bkd.rateCfg.SMTPConnectionsPerMinute)
	bkd.limiters[ip] = l
	return l
}

func (bkd *Backend) getUserOutboundLimiter(userID uuid.UUID) *rate.Limiter {
	bkd.mu.Lock()
	defer bkd.mu.Unlock()
	if l, ok := bkd.userLimit[userID]; ok {
		return l
	}
	r := rate.Every(time.Hour / time.Duration(bkd.rateCfg.OutboundPerUserHour))
	l := rate.NewLimiter(r, bkd.rateCfg.OutboundPerUserHour)
	bkd.userLimit[userID] = l
	return l
}

func (bkd *Backend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	remoteIP, _, _ := net.SplitHostPort(c.Conn().RemoteAddr().String())
	bkd.logger.Info("new smtp connection", "remote_addr", c.Conn().RemoteAddr().String())

	parsedIP := net.ParseIP(remoteIP)

	// Check persistent IP block list (skip for loopback)
	if parsedIP != nil && !parsedIP.IsLoopback() {
		blocked, err := bkd.rateLimitDB.IsIPBlocked(context.Background(), parsedIP)
		if err != nil {
			bkd.logger.Error("failed to check ip block", "ip", remoteIP, "error", err)
		} else if blocked {
			bkd.logger.Warn("smtp connection rejected: ip blocked", "ip", remoteIP)
			return nil, &gosmtp.SMTPError{
				Code:    421,
				Message: "Your IP address is temporarily blocked",
			}
		}
	}

	// In-memory per-IP rate limiting (skip for loopback)
	if parsedIP != nil && !parsedIP.IsLoopback() && bkd.rateCfg.SMTPConnectionsPerMinute > 0 {
		limiter := bkd.getLimiter(remoteIP)
		if !limiter.Allow() {
			bkd.mu.Lock()
			bkd.violations[remoteIP]++
			violations := bkd.violations[remoteIP]
			bkd.mu.Unlock()

			bkd.logger.Warn("smtp connection rate limited", "ip", remoteIP, "violations", violations)

			if bkd.rateCfg.SMTPAutoBlockThreshold > 0 && violations >= bkd.rateCfg.SMTPAutoBlockThreshold {
				until := time.Now().Add(bkd.rateCfg.SMTPAutoBlockDuration)
				if err := bkd.rateLimitDB.AddIPBlock(context.Background(), parsedIP, "auto-blocked: rate limit exceeded", &until); err != nil {
					bkd.logger.Error("failed to auto-block ip", "ip", remoteIP, "error", err)
				} else {
					bkd.logger.Warn("smtp ip auto-blocked", "ip", remoteIP, "until", until)
					bkd.mu.Lock()
					delete(bkd.violations, remoteIP)
					bkd.mu.Unlock()
				}
			}

			return nil, &gosmtp.SMTPError{
				Code:    421,
				Message: "Too many connections from your IP, please try again later",
			}
		}
	}

	return &Session{
		backend:  bkd,
		remoteIP: parsedIP,
	}, nil
}

// Session implements smtp.Session
type Session struct {
	backend  *Backend
	remoteIP net.IP
	from     string
	to       []RecipientInfo

	// Auth state (for authenticated MTA relay)
	authenticated bool
	userID        uuid.UUID
}

func (s *Session) AuthPlain(username, password string) error {
	if !s.backend.cfg.AuthEnabled {
		return &gosmtp.SMTPError{
			Code:    503,
			Message: "AUTH not supported",
		}
	}

	user, err := s.backend.authDB.GetUserByUsername(context.Background(), username)
	if err != nil || !user.IsActive {
		return &gosmtp.SMTPError{
			Code:    535,
			Message: "Authentication failed",
		}
	}

	valid, err := auth.ComparePassword(password, user.PasswordHash)
	if err != nil || !valid {
		return &gosmtp.SMTPError{
			Code:    535,
			Message: "Authentication failed",
		}
	}

	s.authenticated = true
	s.userID = user.ID
	return nil
}

func (s *Session) Mail(from string, opts *gosmtp.MailOptions) error {
	s.backend.logger.Info("smtp mail from", "from", from, "remote_ip", s.remoteIP, "authenticated", s.authenticated)

	if s.authenticated {
		ok, err := s.backend.authDB.IsAuthorizedSendingAddress(context.Background(), s.userID, from)
		if err != nil {
			s.backend.logger.Error("smtp: failed to check sending permission", "from", from, "user_id", s.userID, "error", err)
			return &gosmtp.SMTPError{
				Code:    550,
				Message: "Temporary failure checking sending permission",
			}
		}
		if !ok {
			s.backend.logger.Warn("smtp: unauthorized from address", "from", from, "user_id", s.userID)
			return &gosmtp.SMTPError{
				Code:    550,
				Message: "You do not have permission to send from this address",
			}
		}
	}

	s.from = from
	return nil
}

func (s *Session) Rcpt(to string, opts *gosmtp.RcptOptions) error {
	if s.authenticated {
		s.backend.logger.Debug("smtp rcpt (authenticated)", "to", to, "from", s.from)
		s.to = append(s.to, RecipientInfo{
			Address: to,
		})
		return nil
	}

	mb, mappingID, err := s.backend.db.LookupMailboxByAddress(context.Background(), to)
	if err != nil {
		s.backend.logger.Warn("recipient rejected: mailbox not found", "to", to, "from", s.from, "error", err)
		return &gosmtp.SMTPError{
			Code:         550,
			EnhancedCode: gosmtp.EnhancedCode{5, 1, 1},
			Message:      "User unknown",
		}
	}

	// Greylisting: check (ip, from, to) triplet before accepting the recipient (skip for loopback)
	if s.remoteIP != nil && !s.remoteIP.IsLoopback() && s.backend.rateCfg.GreylistEnabled {
		pass, err := s.backend.rateLimitDB.CheckAndUpdateGreylist(
			context.Background(), s.remoteIP, s.from, to, s.backend.rateCfg.GreylistDelay,
		)
		if err != nil {
			s.backend.logger.Error("greylist check failed", "ip", s.remoteIP, "from", s.from, "to", to, "error", err)
		} else if !pass {
			s.backend.logger.Info("smtp recipient greylisted", "ip", s.remoteIP, "from", s.from, "to", to)
			return &gosmtp.SMTPError{
				Code:         451,
				EnhancedCode: gosmtp.EnhancedCode{4, 7, 1},
				Message:      "Greylisted, please try again later",
			}
		}
	}

	s.backend.logger.Info("recipient accepted", "to", to, "mailbox_id", mb.ID)
	s.to = append(s.to, RecipientInfo{
		Address:   to,
		MailboxID: mb.ID,
		MappingID: mappingID,
	})
	return nil
}

func (s *Session) Data(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		s.backend.logger.Error("failed to read email data", "error", err)
		return err
	}

	if s.authenticated {
		return s.handleOutbound(data)
	}

	return s.handleInbound(data)
}

func (s *Session) handleOutbound(data []byte) error {
	s.backend.logger.Info("smtp outbound relay", "from", s.from, "rcpt_count", len(s.to), "user_id", s.userID)

	// Rate limit check
	if s.backend.rateCfg.OutboundPerUserHour > 0 {
		limiter := s.backend.getUserOutboundLimiter(s.userID)
		if !limiter.Allow() {
			s.backend.logger.Warn("outbound rate limit exceeded", "user_id", s.userID, "from", s.from)
			return &gosmtp.SMTPError{
				Code:    550,
				Message: "Hourly sending limit reached, please try again later",
			}
		}
	}

	recipients := make([]string, 0, len(s.to))
	for _, rcpt := range s.to {
		recipients = append(recipients, rcpt.Address)
	}

	_, err := s.backend.outboundDB.InsertOutboundJob(
		context.Background(),
		nil,
		s.from,
		recipients,
		data,
	)
	if err != nil {
		s.backend.logger.Error("failed to enqueue outbound job", "from", s.from, "error", err)
		return &gosmtp.SMTPError{
			Code:    550,
			Message: "Failed to queue message for delivery",
		}
	}

	s.backend.logger.Info("outbound message queued for delivery", "from", s.from, "rcpt_count", len(s.to))
	return nil
}

func (s *Session) handleInbound(data []byte) error {
	s.backend.logger.Info("smtp data received, processing message", "from", s.from, "rcpt_count", len(s.to))

	for _, rcpt := range s.to {
		ictx := &pipeline.IngestionContext{
			ID:               uuid.New(),
			EmailID:          uuid.New(),
			RemoteIP:         s.remoteIP,
			FromAddress:      s.from,
			ToAddresses:      []string{rcpt.Address},
			RawMessage:       data,
			TargetMailboxID:  rcpt.MailboxID,
			AddressMappingID: rcpt.MappingID,
		}

		s.backend.logger.Info("queueing ingestion", "ingestion_id", ictx.ID, "from", s.from, "to", rcpt.Address)

		go func(ctx *pipeline.IngestionContext) {
			if err := s.backend.pipeline.Process(context.Background(), ctx); err != nil {
				s.backend.logger.Error("pipeline processing failed", "ingestion_id", ctx.ID, "error", err)
			}
		}(ictx)
	}

	return nil
}

func (s *Session) Reset() {
	s.backend.logger.Info("smtp session reset", "from", s.from)
	s.from = ""
	s.to = nil
}

func (s *Session) Logout() error {
	s.backend.logger.Info("smtp session logout", "from", s.from)
	return nil
}

// CreateServers initializes and starts the SMTP servers on the configured ports
func CreateServers(cfg config.SMTPConfig, rateCfg config.RateLimitConfig, mailDB db.MailDB, rateLimitDB db.RateLimitDB, p *pipeline.Pipeline, authDB SMTPAuthDB, outboundDB SMTPOutboundDB) ([]*gosmtp.Server, error) {
	var servers []*gosmtp.Server
	be := &Backend{
		cfg:         cfg,
		rateCfg:     rateCfg,
		db:          mailDB,
		rateLimitDB: rateLimitDB,
		pipeline:    p,
		authDB:      authDB,
		outboundDB:  outboundDB,
		logger:      slog.With("service", "smtp"),
		limiters:    make(map[string]*rate.Limiter),
		violations:  make(map[string]int),
		userLimit:   make(map[uuid.UUID]*rate.Limiter),
	}

	var tlsConfig *tls.Config
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		be.logger.Info("loading smtp tls certificates", "cert", cfg.TLSCertFile, "key", cfg.TLSKeyFile)
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS key pair: %w", err)
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	} else {
		be.logger.Warn("no smtp tls certificates provided, STARTTLS will not be available")
	}

	for _, port := range cfg.Ports {
		s := gosmtp.NewServer(be)
		s.Addr = fmt.Sprintf(":%d", port)
		s.Domain = cfg.Domain
		s.ReadTimeout = cfg.ReadTimeout
		s.WriteTimeout = cfg.WriteTimeout
		s.MaxMessageBytes = cfg.MaxMessageSize
		s.MaxRecipients = cfg.MaxRecipients
		s.AllowInsecureAuth = false
		s.TLSConfig = tlsConfig

		servers = append(servers, s)
	}

	return servers, nil
}

func StartServer(srv *gosmtp.Server) error {
	log := slog.With("service", "smtp")
	tlsStatus := "disabled"
	if srv.TLSConfig != nil {
		tlsStatus = "enabled"
	}
	log.Info("Starting SMTP server", "addr", srv.Addr, "starttls", tlsStatus)

	var err error
	if strings.HasSuffix(srv.Addr, ":465") || strings.HasSuffix(srv.Addr, ":4650") {
		if srv.TLSConfig == nil {
			log.Error("Implicit TLS requested but no certificates provided. Skipping port.", "addr", srv.Addr)
			return err
		}
		log.Info("Using implicit TLS for port", "addr", srv.Addr)
		err = srv.ListenAndServeTLS()
	} else {
		err = srv.ListenAndServe()
	}

	if err != nil {
		log.Error("SMTP server failed", "addr", srv.Addr, "error", err)
	}
	return err
}
