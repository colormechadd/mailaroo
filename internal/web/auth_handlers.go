package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/colormechadd/mailaroo/internal/outbound"
	"github.com/colormechadd/mailaroo/pkg/auth"
	"github.com/colormechadd/mailaroo/templates"
	"github.com/gorilla/csrf"
)

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	templates.LoginPage("", csrf.Token(r)).Render(r.Context(), w)
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	if remoteIP == "" {
		remoteIP = r.RemoteAddr
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	// recordFailure consumes a token from the per-IP limiter and, if exhausted,
	// returns true to signal that the caller should reject with 429.
	recordFailure := func() bool {
		limiter := s.loginLimiter(remoteIP)
		if !limiter.Allow() {
			s.logger.Warn("login rate limited", "ip", remoteIP)
			http.Error(w, "Too many failed login attempts, please try again later", http.StatusTooManyRequests)
			return true
		}
		return false
	}

	user, err := s.DB.GetUserByUsername(r.Context(), username)
	if err != nil || !user.IsActive {
		s.logger.Warn("login failed: user not found or inactive", "username", username)
		if recordFailure() {
			return
		}
		templates.LoginPage("Invalid credentials", csrf.Token(r)).Render(r.Context(), w)
		return
	}

	match, err := auth.ComparePassword(password, user.PasswordHash)
	if err != nil || !match {
		s.logger.Warn("login failed: incorrect password", "username", username)
		if recordFailure() {
			return
		}
		templates.LoginPage("Invalid credentials", csrf.Token(r)).Render(r.Context(), w)
		return
	}

	if user.TOTPEnabled {
		token := generateToken()
		expires := time.Now().Add(5 * time.Minute)
		if err := s.DB.CreateTOTPPending(r.Context(), user.ID, token, expires); err != nil {
			s.logger.Error("failed to create TOTP pending session", "user_id", user.ID, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "mailaroo_totp_pending",
			Value:    token,
			Expires:  expires,
			HttpOnly: true,
			Secure:   s.secureCookies,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
		})
		http.Redirect(w, r, "/verify-totp", http.StatusSeeOther)
		return
	}

	token := generateToken()
	s.logger.Info("Expire seconds", "seconds", s.Config.Web.SessionExpirationSeconds)
	expires := time.Now().Add(time.Duration(s.Config.Web.SessionExpirationSeconds) * time.Second)
	if err := s.DB.CreateWebmailSession(r.Context(), user.ID, token, r.RemoteAddr, r.UserAgent(), expires); err != nil {
		s.logger.Error("failed to create session", "user_id", user.ID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "mailaroo_session",
		Value:    token,
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("mailaroo_session")
	if err == nil {
		if err := s.DB.ExpireWebmailSession(r.Context(), cookie.Value); err != nil {
			s.logger.Error("failed to expire session on logout", "error", err)
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "mailaroo_session",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleForgotPasswordGet(w http.ResponseWriter, r *http.Request) {
	templates.ForgotPasswordPage("", "", csrf.Token(r)).Render(r.Context(), w)
}

func (s *Server) handleForgotPasswordPost(w http.ResponseWriter, r *http.Request) {
	remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	if remoteIP == "" {
		remoteIP = r.RemoteAddr
	}

	limiter := s.forgotPasswordLimiter(remoteIP)
	if !limiter.Allow() {
		s.logger.Warn("forgot password rate limited", "ip", remoteIP)
		http.Error(w, "Too many password reset requests, please try again later", http.StatusTooManyRequests)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))

	user, err := s.DB.GetUserByUsername(r.Context(), username)
	if err != nil || !user.IsActive {
		templates.ForgotPasswordPage("", "If that username exists and has a recovery email set, a password reset link has been sent.", csrf.Token(r)).Render(r.Context(), w)
		return
	}

	if user.RecoveryEmail == "" {
		s.logger.Warn("password reset requested for user with no recovery email", "username", username)
		templates.ForgotPasswordPage("", "If that username exists and has a recovery email set, a password reset link has been sent.", csrf.Token(r)).Render(r.Context(), w)
		return
	}

	token := generateToken()
	expires := time.Now().Add(15 * time.Minute)
	if err := s.DB.CreatePasswordReset(r.Context(), user.ID, token, expires); err != nil {
		s.logger.Error("failed to create password reset token", "user_id", user.ID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.publicURL(r), token)
	from := fmt.Sprintf("noreply@%s", s.Config.SMTP.Domain)
	addrs, err := s.DB.GetActiveSendingAddresses(r.Context(), user.ID)
	if err == nil && len(addrs) > 0 {
		from = addrs[0].Address
	}
	msg := outbound.Message{
		From:     from,
		To:       []string{user.RecoveryEmail},
		Subject:  "Password Reset - MAILAROO",
		TextBody: fmt.Sprintf("A password reset was requested for your MAILAROO account (%s).\n\nTo reset your password, click the link below (valid for 15 minutes):\n\n%s\n\nIf you did not request this, please ignore this email.", user.Username, resetURL),
	}

	if _, err := s.Sender.SendMessage(msg); err != nil {
		s.logger.Error("failed to send password reset email", "user_id", user.ID, "recovery_email", user.RecoveryEmail, "error", err)
		// Token is already created but email failed - still show success for security
	}

	s.logger.Info("password reset email sent", "username", username, "recovery_email", user.RecoveryEmail)
	templates.ForgotPasswordPage("", "If that username exists and has a recovery email set, a password reset link has been sent.", csrf.Token(r)).Render(r.Context(), w)
}

func (s *Server) handleResetPasswordGet(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing reset token", http.StatusBadRequest)
		return
	}
	templates.ResetPasswordPage("", token, csrf.Token(r)).Render(r.Context(), w)
}

func (s *Server) handleResetPasswordPost(w http.ResponseWriter, r *http.Request) {
	remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	if remoteIP == "" {
		remoteIP = r.RemoteAddr
	}

	limiter := s.resetPasswordLimiter(remoteIP)
	if !limiter.Allow() {
		s.logger.Warn("reset password rate limited", "ip", remoteIP)
		http.Error(w, "Too many reset attempts, please try again later", http.StatusTooManyRequests)
		return
	}

	token := r.FormValue("token")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm_password")

	if token == "" {
		templates.ResetPasswordPage("Missing reset token.", token, csrf.Token(r)).Render(r.Context(), w)
		return
	}
	if password == "" || len(password) < 8 {
		templates.ResetPasswordPage("Password must be at least 8 characters.", token, csrf.Token(r)).Render(r.Context(), w)
		return
	}
	if password != confirm {
		templates.ResetPasswordPage("Passwords do not match.", token, csrf.Token(r)).Render(r.Context(), w)
		return
	}

	reset, err := s.DB.GetPasswordResetByToken(r.Context(), token)
	if err != nil {
		s.logger.Warn("invalid or expired password reset token", "error", err)
		templates.ResetPasswordPage("Invalid or expired reset token. Please request a new password reset.", token, csrf.Token(r)).Render(r.Context(), w)
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		s.logger.Error("failed to hash new password", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := s.DB.UpdateUserPassword(r.Context(), reset.UserID, hash); err != nil {
		s.logger.Error("failed to update password", "user_id", reset.UserID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := s.DB.DeletePasswordReset(r.Context(), token); err != nil {
		s.logger.Error("failed to delete used password reset token", "error", err)
	}

	s.logger.Info("password reset successful", "user_id", reset.UserID)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// publicURL builds an absolute base URL from the incoming request.
func (s *Server) publicURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || s.secureCookies {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func generateToken() string {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic("failed to generate random token: " + err.Error())
	}
	return hex.EncodeToString(b)
}
