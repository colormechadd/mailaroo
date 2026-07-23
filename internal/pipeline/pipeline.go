package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/colormechadd/mailaroo/internal/classifier"
	"github.com/colormechadd/mailaroo/internal/config"
	"github.com/colormechadd/mailaroo/internal/db"
	"github.com/colormechadd/mailaroo/internal/mail"
	"github.com/colormechadd/mailaroo/internal/rspamd"
	"github.com/colormechadd/mailaroo/internal/storage"
	"github.com/colormechadd/mailaroo/pkg/models"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

type Stage string

const (
	StageConnect  Stage = "connect"
	StageMailFrom Stage = "mail_from"
	StageRcptTo   Stage = "rcpt_to"
	StageData     Stage = "data"
)

type StepStatus string

const (
	StatusPass        StepStatus = "pass"
	StatusFail        StepStatus = "fail"
	StatusNeutral     StepStatus = "neutral"
	StatusQuarantined StepStatus = "quarantined"
	StatusError       StepStatus = "error"
	StatusSkipped     StepStatus = "skipped"
	StatusNone        StepStatus = "none"
)

type Event struct {
	UserID    uuid.UUID
	MailboxID uuid.UUID
	Type      string
}

type Broadcaster interface {
	Broadcast(event Event)
}

type RejectError struct {
	Code    int
	Message string
}

func (e *RejectError) Error() string {
	return e.Message
}

type Step func(ctx context.Context, p *Pipeline, ictx *IngestionContext) (StepStatus, any, error)

type StepRegistration struct {
	Name  string
	Stage Stage
	Fn    Step
}

type Pipeline struct {
	logger     *slog.Logger
	cfg        *config.Config
	db         db.PipelineDB
	storage    storage.Storage
	hub        Broadcaster
	mail       *mail.Service
	rspamd     *rspamd.Client
	classifier classifier.Classifier
	steps      []StepRegistration

	mu              sync.Mutex
	limiters        map[string]*rate.Limiter
	limiterLastSeen map[string]time.Time
	violations      map[string]int
	cleanupTick     int
}

const limiterCleanupInterval = 100
const limiterStaleAge = 10 * time.Minute

func NewPipeline(cfg *config.Config, db db.PipelineDB, storage storage.Storage, hub Broadcaster, mailSvc *mail.Service, rspamdClient *rspamd.Client, classifier classifier.Classifier) *Pipeline {
	p := &Pipeline{
		logger:     slog.With("service", "pipeline"),
		cfg:        cfg,
		db:         db,
		storage:    storage,
		hub:        hub,
		mail:       mailSvc,
		rspamd:     rspamdClient,
		classifier: classifier,
		limiters:        make(map[string]*rate.Limiter),
		limiterLastSeen: make(map[string]time.Time),
		violations:      make(map[string]int),
	}

	p.Register(StageConnect, "ip_block", CheckIPBlock)
	p.Register(StageConnect, "ip_rate_limit", CheckRateLimit)
	p.Register(StageConnect, "rbl", ValidateRBL)
	p.Register(StageRcptTo, "from_address_block", CheckBlockingRules)
	p.Register(StageData, "validate_sender", ValidateSender)
	p.Register(StageData, "strip_tracking_pixels", StripTrackingPixels)
	p.Register(StageData, "deliver", Deliver)
	p.Register(StageData, "parse_dsn", ParseDSN)
	p.Register(StageData, "check_spam", CheckSpam)
	p.Register(StageData, "classify_email", ClassifyEmail)
	p.Register(StageData, "apply_filter_rules", ApplyFilterRules)
	p.Register(StageData, "finalize", Finalize)
	p.Register(StageData, "notify", Notify)

	return p
}

type IngestionContext struct {
	ID                  uuid.UUID
	RemoteIP            net.IP
	FromAddress         string
	ToAddresses         []string
	RawMessage          []byte
	TargetMailboxID     uuid.UUID
	AddressMappingID    uuid.UUID
	StorageKey          string
	EmailID             uuid.UUID
	MatchedFilterRuleID *uuid.UUID
	FilterAction        string
	IsRead              bool
	Category            string
}

func (p *Pipeline) Register(stage Stage, name string, fn Step) {
	p.steps = append(p.steps, StepRegistration{Name: name, Stage: stage, Fn: fn})
}

func (p *Pipeline) NewIngestion(ctx context.Context, ictx *IngestionContext) error {
	ingestion := &models.Ingestion{
		ID:     ictx.ID,
		Status: "processing",
	}
	if ictx.FromAddress != "" {
		ingestion.FromAddress = &ictx.FromAddress
	}
	if len(ictx.ToAddresses) > 0 {
		ingestion.ToAddress = &ictx.ToAddresses[0]
	}
	return p.db.CreateIngestion(ctx, ingestion)
}

func (p *Pipeline) RunStage(ctx context.Context, stage Stage, ictx *IngestionContext) error {
	for _, step := range p.steps {
		if step.Stage != stage {
			continue
		}
		status, stepErr := p.runStep(ctx, ictx, step.Name, step.Fn)
		switch status {
		case StatusFail, StatusError:
			if err := p.db.UpdateIngestionStatus(ctx, ictx.ID, "rejected"); err != nil {
				p.logger.Error("failed to update ingestion status", "ingestion_id", ictx.ID, "error", err)
			}
			if stepErr != nil {
				return stepErr
			}
			return &RejectError{Code: 550, Message: fmt.Sprintf("pipeline step %q rejected", step.Name)}
		}
	}

	if err := p.db.UpdateIngestionStatus(ctx, ictx.ID, "accepted"); err != nil {
		p.logger.Error("failed to update ingestion status", "ingestion_id", ictx.ID, "error", err)
	}
	return nil
}

func (p *Pipeline) Process(ctx context.Context, ictx *IngestionContext) error {
	p.logger.Info("processing ingestion", "ingestion_id", ictx.ID, "from", ictx.FromAddress, "mailbox_id", ictx.TargetMailboxID)

	ingestion := &models.Ingestion{
		ID:          ictx.ID,
		FromAddress: &ictx.FromAddress,
		Status:      "processing",
	}
	if len(ictx.ToAddresses) > 0 {
		ingestion.ToAddress = &ictx.ToAddresses[0]
	}

	if err := p.db.CreateIngestion(ctx, ingestion); err != nil {
		p.logger.Error("failed to create ingestion record", "ingestion_id", ictx.ID, "error", err)
		return err
	}

	for _, step := range p.steps {
		if step.Stage != StageData {
			continue
		}
		status, _ := p.runStep(ctx, ictx, step.Name, step.Fn)
		if status == StatusFail {
			p.logger.Warn("ingestion rejected", "ingestion_id", ictx.ID, "step", step.Name)
			return p.db.UpdateIngestionStatus(ctx, ictx.ID, "rejected")
		}
		if status == StatusError {
			p.logger.Error("ingestion failed", "ingestion_id", ictx.ID, "step", step.Name)
			return p.db.UpdateIngestionStatus(ctx, ictx.ID, "failed")
		}
	}

	p.logger.Info("ingestion completed successfully", "ingestion_id", ictx.ID)
	return p.db.UpdateIngestionStatus(ctx, ictx.ID, "accepted")
}

func (p *Pipeline) runStep(ctx context.Context, ictx *IngestionContext, name string, fn Step) (StepStatus, error) {
	start := time.Now()
	status, details, stepErr := fn(ctx, p, ictx)
	duration := time.Since(start)

	detailsJSON, _ := json.Marshal(details)
	if stepErr != nil {
		p.logger.Error("pipeline step error", "ingestion_id", ictx.ID, "step", name, "status", status, "error", stepErr)
		if details == nil {
			detailsJSON, _ = json.Marshal(map[string]string{"error": stepErr.Error()})
		}
	}

	step := &models.IngestionStep{
		ID:          uuid.New(),
		IngestionID: ictx.ID,
		StepName:    name,
		Status:      string(status),
		Details:     detailsJSON,
		DurationMS:  int(duration.Milliseconds()),
	}
	if dbErr := p.db.CreateIngestionStep(ctx, step); dbErr != nil {
		p.logger.Error("failed to record ingestion step", "ingestion_id", ictx.ID, "step", name, "error", dbErr)
	}

	return status, stepErr
}
