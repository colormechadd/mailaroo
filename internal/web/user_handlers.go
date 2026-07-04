package web

import (
	"io"
	"mime"
	"net/http"
	"net/mail"
	"strings"

	"github.com/colormechadd/mailaroo/pkg/auth"
	"github.com/colormechadd/mailaroo/pkg/models"
	"github.com/colormechadd/mailaroo/templates"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	attID, err := uuid.Parse(chi.URLParam(r, "attachmentID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	att, err := s.DB.GetAttachmentByIDForUser(r.Context(), attID, user.ID)
	if err != nil {
		s.logger.Error("attachment not found or forbidden", "att_id", attID, "user_id", user.ID, "error", err)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	rc, err := s.Storage.Get(r.Context(), att.StorageKey)
	if err != nil {
		s.logger.Error("failed to fetch attachment", "key", att.StorageKey, "error", err)
		http.Error(w, "Failed to load", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	bodyReader, err := s.Mail.DecompressReader(rc, att.StorageKey)
	if err != nil {
		s.logger.Error("failed to decompress attachment", "key", att.StorageKey, "error", err)
		http.Error(w, "Failed to load", http.StatusInternalServerError)
		return
	}
	if closer, ok := bodyReader.(io.Closer); ok {
		defer closer.Close()
	}

	s.logger.Info("content type", "type", att.ContentType)
	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": att.Filename}))
	io.Copy(w, bodyReader)
}

func (s *Server) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)

	mailboxes, err := s.DB.GetMailboxesByUserID(r.Context(), user.ID)
	if err != nil {
		s.logger.Error("failed to fetch mailboxes", "user_id", user.ID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	sendingAddresses, err := s.DB.GetActiveSendingAddresses(r.Context(), user.ID)
	if err != nil {
		s.logger.Error("failed to fetch sending addresses", "user_id", user.ID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	sessions, err := s.DB.ListActiveSessions(r.Context(), user.ID)
	if err != nil {
		s.logger.Error("failed to fetch active sessions", "user_id", user.ID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	currentMailboxID := uuid.Nil
	if len(mailboxes) > 0 {
		currentMailboxID = mailboxes[0].ID
	}
	s.render(w, r, user, mailboxes, currentMailboxID, "all", nil, templates.UserInfo(user, mailboxes, sendingAddresses, sessions), "Account")
}

func (s *Server) handleCancelSession(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	if err := s.DB.ExpireWebmailSessionByID(r.Context(), sessionID); err != nil {
		s.logger.Error("failed to cancel session", "session_id", sessionID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	sessions, err := s.DB.ListActiveSessions(r.Context(), user.ID)
	if err != nil {
		s.logger.Error("failed to list sessions after cancel", "user_id", user.ID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	templates.SessionsList(sessions).Render(r.Context(), w)
}

func (s *Server) handleUpdateDisplayName(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	saIDRaw := chi.URLParam(r, "saID")
	saID, err := uuid.Parse(saIDRaw)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	displayName := r.FormValue("display_name")

	err = s.DB.UpdateSendingAddressDisplayName(r.Context(), saID, user.ID, displayName)
	if err != nil {
		s.logger.Error("failed to update display name", "user_id", user.ID, "sa_id", saID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if newPassword == "" || len(newPassword) < 8 {
		templates.ChangePasswordMessage("New password must be at least 8 characters.", true).Render(r.Context(), w)
		return
	}
	if newPassword != confirmPassword {
		templates.ChangePasswordMessage("New passwords do not match.", true).Render(r.Context(), w)
		return
	}

	match, err := auth.ComparePassword(currentPassword, user.PasswordHash)
	if err != nil || !match {
		templates.ChangePasswordMessage("Current password is incorrect.", true).Render(r.Context(), w)
		return
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		s.logger.Error("failed to hash new password", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := s.DB.UpdateUserPassword(r.Context(), user.ID, hash); err != nil {
		s.logger.Error("failed to update password", "user_id", user.ID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	s.logger.Info("password changed", "user_id", user.ID)
	templates.ChangePasswordMessage("Password updated successfully.", false).Render(r.Context(), w)
}

func (s *Server) handleUserUpdateRecoveryEmail(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)

	currentPassword := r.FormValue("current_password")
	email := strings.TrimSpace(r.FormValue("recovery_email"))

	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			templates.ChangePasswordMessage("Invalid email address.", true).Render(r.Context(), w)
			return
		}
	}

	match, err := auth.ComparePassword(currentPassword, user.PasswordHash)
	if err != nil || !match {
		templates.ChangePasswordMessage("Current password is incorrect.", true).Render(r.Context(), w)
		return
	}

	if err := s.DB.UpdateUserRecoveryEmail(r.Context(), user.ID, email); err != nil {
		s.logger.Error("failed to update recovery email", "user_id", user.ID, "error", err)
		templates.ChangePasswordMessage("Failed to update recovery email.", true).Render(r.Context(), w)
		return
	}

	s.logger.Info("recovery email updated", "user_id", user.ID, "email", email)
	templates.ChangePasswordMessage("Recovery email updated successfully.", false).Render(r.Context(), w)
}
