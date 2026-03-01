package web

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/colormechadd/maileroo/internal/config"
	"github.com/colormechadd/maileroo/internal/db"
	"github.com/colormechadd/maileroo/internal/storage"
	"github.com/colormechadd/maileroo/pkg/auth"
	"github.com/colormechadd/maileroo/pkg/models"
	"github.com/colormechadd/maileroo/templates"
	"github.com/emersion/go-message/mail"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/microcosm-cc/bluemonday"
)

type Server struct {
	cfg     config.Config
	db      db.WebDB
	storage storage.Storage
	policy  *bluemonday.Policy
}

func NewServer(cfg config.Config, webDB db.WebDB, storage storage.Storage) *Server {
	p := bluemonday.UGCPolicy()
	p.AllowStyling()

	return &Server{
		cfg:     cfg,
		db:      webDB,
		storage: storage,
		policy:  p,
	}
}

func (s *Server) Routes() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	fs := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	r.Get("/login", templ.Handler(templates.LoginPage("")).ServeHTTP)
	r.Post("/login", s.handleLoginPost)
	r.Post("/logout", s.handleLogout)

	r.Group(func(r chi.Router) {
		r.Use(s.AuthMiddleware)
		r.Get("/", s.handleDashboard)
		r.Get("/mailbox/{mailboxID}", s.handleMailboxView)
		r.Get("/email/{emailID}", s.handleEmailView)
		r.Get("/email/{emailID}/headers", s.handleEmailHeaders)
		r.Get("/email/{emailID}/pipeline", s.handleEmailPipeline)
		r.Get("/attachment/{attachmentID}", s.handleAttachmentDownload)
	})

	return r
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, user *models.User, mailboxes []models.Mailbox, currentMailboxID uuid.UUID, content templ.Component) {
	if r.Header.Get("HX-Request") == "true" {
		content.Render(r.Context(), w)
		return
	}
	templates.Dashboard(user, mailboxes, currentMailboxID, content).Render(r.Context(), w)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxes, err := s.db.GetMailboxesByUserID(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if len(mailboxes) > 0 {
		http.Redirect(w, r, "/mailbox/"+mailboxes[0].ID.String(), http.StatusSeeOther)
		return
	}

	s.render(w, r, user, mailboxes, uuid.Nil, templates.MailboxContent(mailboxes, uuid.Nil, nil))
}

func (s *Server) handleMailboxView(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxID, _ := uuid.Parse(chi.URLParam(r, "mailboxID"))

	mailboxes, _ := s.db.GetMailboxesByUserID(r.Context(), user.ID)
	emails, _ := s.db.GetEmailsByMailboxID(r.Context(), mailboxID, 50, 0)

	s.render(w, r, user, mailboxes, mailboxID, templates.MailboxContent(mailboxes, mailboxID, emails))
}

func (s *Server) handleEmailView(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	emailID, _ := uuid.Parse(chi.URLParam(r, "emailID"))

	email, err := s.db.GetEmailByIDForUser(r.Context(), emailID, user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	attachments, _ := s.db.GetAttachmentsByEmailID(r.Context(), emailID)
	
	content, isHTML := s.fetchEmailBody(r.Context(), email)

	mailboxes, _ := s.db.GetMailboxesByUserID(r.Context(), user.ID)
	s.render(w, r, user, mailboxes, email.MailboxID, templates.EmailDetail(email, attachments, content, isHTML))
}

func (s *Server) handleEmailHeaders(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	emailID, _ := uuid.Parse(chi.URLParam(r, "emailID"))
	email, err := s.db.GetEmailByIDForUser(r.Context(), emailID, user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	
	headers := s.fetchEmailHeaders(r.Context(), email)

	mailboxes, _ := s.db.GetMailboxesByUserID(r.Context(), user.ID)
	s.render(w, r, user, mailboxes, email.MailboxID, templates.EmailHeaders(email, headers))
}

func (s *Server) handleEmailPipeline(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	emailID, _ := uuid.Parse(chi.URLParam(r, "emailID"))
	email, err := s.db.GetEmailByIDForUser(r.Context(), emailID, user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	steps, err := s.db.GetIngestionStepsByEmailID(r.Context(), emailID, user.ID)
	if err != nil {
		slog.Error("failed to fetch ingestion steps", "email_id", emailID, "error", err)
	}

	for i := range steps {
		if len(steps[i].Details) > 0 && string(steps[i].Details) != "null" {
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, steps[i].Details, "", "  "); err == nil {
				steps[i].Details = pretty.Bytes()
			}
		}
	}

	mailboxes, _ := s.db.GetMailboxesByUserID(r.Context(), user.ID)
	s.render(w, r, user, mailboxes, email.MailboxID, templates.EmailPipeline(email, steps))
}

func (s *Server) fetchEmailBody(ctx context.Context, email *models.Email) (string, bool) {
	rc, err := s.storage.Get(ctx, email.StorageKey)
	if err != nil {
		return "Failed to load content", false
	}
	defer rc.Close()

	var bodyReader io.Reader = rc
	if strings.HasSuffix(email.StorageKey, ".zst") {
		zr, _ := zstd.NewReader(rc)
		if zr != nil {
			defer zr.Close()
			bodyReader = zr
		}
	} else if strings.HasSuffix(email.StorageKey, ".gz") {
		gr, _ := gzip.NewReader(rc)
		if gr != nil {
			defer gr.Close()
			bodyReader = gr
		}
	}

	mr, err := mail.CreateReader(bodyReader)
	if err != nil {
		b, _ := io.ReadAll(bodyReader)
		return string(b), false
	}
	defer mr.Close()

	var content string
	var isHTML bool
	for {
		p, err := mr.NextPart()
		if err != nil { break }

		var ct string
		switch h := p.Header.(type) {
		case *mail.InlineHeader: ct, _, _ = h.ContentType()
		case *mail.AttachmentHeader: ct, _, _ = h.ContentType()
		}

		if ct == "text/html" {
			b, _ := io.ReadAll(p.Body)
			content = s.policy.Sanitize(string(b))
			isHTML = true
			break
		}
		if ct == "text/plain" && content == "" {
			b, _ := io.ReadAll(p.Body)
			content = string(b)
		}
	}
	return content, isHTML
}

func (s *Server) fetchEmailHeaders(ctx context.Context, email *models.Email) string {
	rc, err := s.storage.Get(ctx, email.StorageKey)
	if err != nil { return "" }
	defer rc.Close()

	var bodyReader io.Reader = rc
	if strings.HasSuffix(email.StorageKey, ".zst") {
		zr, _ := zstd.NewReader(rc)
		if zr != nil { defer zr.Close(); bodyReader = zr }
	} else if strings.HasSuffix(email.StorageKey, ".gz") {
		gr, _ := gzip.NewReader(rc)
		if gr != nil { defer gr.Close(); bodyReader = gr }
	}

	mr, err := mail.CreateReader(bodyReader)
	if err != nil { return "" }
	defer mr.Close()

	var sb strings.Builder
	fields := mr.Header.Fields()
	for fields.Next() {
		sb.WriteString(fmt.Sprintf("%s: %s\n", fields.Key(), fields.Value()))
	}
	return sb.String()
}

func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("maileroo_session")
		if err != nil {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session, err := s.db.GetWebmailSession(r.Context(), cookie.Value)
		if err != nil || session.ExpiresDatetime.Before(time.Now()) {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		user, err := s.db.GetUserByID(r.Context(), session.UserID)
		if err != nil || !user.IsActive {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), "user", user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := s.db.GetUserByUsername(r.Context(), username)
	if err != nil || !user.IsActive {
		templates.LoginPage("Invalid credentials").Render(r.Context(), w)
		return
	}

	match, err := auth.ComparePassword(password, user.PasswordHash)
	if err != nil || !match {
		templates.LoginPage("Invalid credentials").Render(r.Context(), w)
		return
	}

	token := generateToken()
	expires := time.Now().Add(24 * time.Hour)
	if err := s.db.CreateWebmailSession(r.Context(), user.ID, token, r.RemoteAddr, r.UserAgent(), expires); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "maileroo_session",
		Value:    token,
		Expires:  expires,
		HttpOnly: true,
		Path:     "/",
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("maileroo_session")
	if err == nil {
		s.db.ExpireWebmailSession(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "maileroo_session",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HttpOnly: true,
		Path:     "/",
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	attID, _ := uuid.Parse(chi.URLParam(r, "attachmentID"))

	att, err := s.db.GetAttachmentByIDForUser(r.Context(), attID, user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	rc, err := s.storage.Get(r.Context(), att.StorageKey)
	if err != nil {
		http.Error(w, "Failed to load", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	var bodyReader io.Reader = rc
	if strings.HasSuffix(att.StorageKey, ".zst") {
		zr, _ := zstd.NewReader(rc)
		if zr != nil { defer zr.Close(); bodyReader = zr }
	} else if strings.HasSuffix(att.StorageKey, ".gz") {
		gr, _ := gzip.NewReader(rc)
		if gr != nil { defer gr.Close(); bodyReader = gr }
	}

	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", att.Filename))
	io.Copy(w, bodyReader)
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
