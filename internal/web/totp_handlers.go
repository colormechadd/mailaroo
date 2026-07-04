package web

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"net/http"
	"time"

	"github.com/colormechadd/mailaroo/pkg/auth"
	"github.com/colormechadd/mailaroo/pkg/models"
	"github.com/colormechadd/mailaroo/templates"
	"github.com/gorilla/csrf"
	qrcode "github.com/skip2/go-qrcode"
)

// --- Login TOTP verification ---

func (s *Server) handleTOTPVerifyGet(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("mailaroo_totp_pending")
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	pending, err := s.DB.GetTOTPPendingByToken(r.Context(), cookie.Value)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	_ = pending
	templates.TOTPVerifyPage("", csrf.Token(r)).Render(r.Context(), w)
}

func (s *Server) handleTOTPVerifyPost(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("mailaroo_totp_pending")
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	pending, err := s.DB.GetTOTPPendingByToken(r.Context(), cookie.Value)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	user, err := s.DB.GetUserByID(r.Context(), pending.UserID)
	if err != nil || !user.IsActive || !user.TOTPEnabled || user.TOTPSecret == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	code := r.FormValue("code")
	valid := auth.ValidateTOTPCode(code, *user.TOTPSecret)

	if !valid {
		idx := auth.VerifyBackupCode(code, user.TOTPBackupCodes)
		if idx < 0 {
			templates.TOTPVerifyPage("Invalid code. Please try again.", csrf.Token(r)).Render(r.Context(), w)
			return
		}
		// Consume the backup code — hard fail if removal fails to prevent reuse
		if err := s.DB.RemoveTOTPBackupCode(r.Context(), user.ID, idx); err != nil {
			s.logger.Error("failed to remove backup code", "user_id", user.ID, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	_ = s.DB.DeleteTOTPPending(r.Context(), cookie.Value)

	http.SetCookie(w, &http.Cookie{
		Name:     "mailaroo_totp_pending",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})

	token := generateToken()
	expires := time.Now().Add(time.Duration(s.Config.Web.SessionExpirationSeconds) * time.Second)
	if err := s.DB.CreateWebmailSession(r.Context(), user.ID, token, r.RemoteAddr, r.UserAgent(), expires); err != nil {
		s.logger.Error("failed to create session after TOTP", "user_id", user.ID, "error", err)
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

// --- User settings TOTP setup ---

func (s *Server) handleTOTPSetupGet(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)

	key, err := auth.GenerateTOTPKey(user.Username, "Mailaroo")
	if err != nil {
		s.logger.Error("failed to generate TOTP key", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Store the secret server-side; never trust it back from the client.
	s.storePendingTOTPSetup(user.ID, key.Secret())

	img, err := key.Image(200, 200)
	if err != nil {
		s.logger.Error("failed to generate TOTP QR image", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	qrDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	templates.TOTPSetupFragment(key.Secret(), qrDataURL).Render(r.Context(), w)
}

func (s *Server) handleTOTPEnablePost(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)

	// Prevent silent re-enrollment: require disable-then-enable flow.
	if user.TOTPEnabled {
		http.Error(w, "Two-factor authentication is already enabled. Disable it first before re-enrolling.", http.StatusBadRequest)
		return
	}

	// Retrieve the server-side secret; never accept one from the client.
	secret, ok := s.getPendingTOTPSetup(user.ID)
	if !ok {
		http.Error(w, "Setup session expired. Please start again.", http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")

	if !auth.ValidateTOTPCode(code, secret) {
		// Re-generate QR for the stored secret so the user can try again.
		qr, err := qrcode.New("otpauth://totp/Mailaroo:"+user.Username+"?secret="+secret+"&issuer=Mailaroo", qrcode.Medium)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		png, err := qr.PNG(200)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		qrDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		// Re-store so the pending entry remains valid for the next attempt.
		s.storePendingTOTPSetup(user.ID, secret)
		templates.TOTPSetupFragment(secret, qrDataURL).Render(r.Context(), w)
		return
	}

	plainCodes, hashedCodes, err := auth.GenerateBackupCodes()
	if err != nil {
		s.logger.Error("failed to generate backup codes", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if err := s.DB.SaveTOTPSetup(r.Context(), user.ID, secret, hashedCodes); err != nil {
		s.logger.Error("failed to save TOTP setup", "user_id", user.ID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	s.deletePendingTOTPSetup(user.ID)
	templates.TOTPBackupCodesFragment(plainCodes).Render(r.Context(), w)
}

func (s *Server) handleTOTPDisablePost(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)

	password := r.FormValue("password")
	match, err := auth.ComparePassword(password, user.PasswordHash)
	if err != nil || !match {
		templates.TOTPEnabledSection("Incorrect password.").Render(r.Context(), w)
		return
	}

	if err := s.DB.DisableTOTP(r.Context(), user.ID); err != nil {
		s.logger.Error("failed to disable TOTP", "user_id", user.ID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	templates.TOTPDisabledFragment("").Render(r.Context(), w)
}

func (s *Server) handleTOTPRegenerateBackupCodes(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)

	if !user.TOTPEnabled || user.TOTPSecret == nil {
		http.Error(w, "2FA not enabled", http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	if !auth.ValidateTOTPCode(code, *user.TOTPSecret) && auth.VerifyBackupCode(code, user.TOTPBackupCodes) < 0 {
		http.Error(w, "Invalid TOTP code", http.StatusForbidden)
		return
	}

	plainCodes, hashedCodes, err := auth.GenerateBackupCodes()
	if err != nil {
		s.logger.Error("failed to generate backup codes", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if err := s.DB.RegenerateTOTPBackupCodes(r.Context(), user.ID, hashedCodes); err != nil {
		s.logger.Error("failed to regenerate backup codes", "user_id", user.ID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	templates.TOTPBackupCodesFragment(plainCodes).Render(r.Context(), w)
}
