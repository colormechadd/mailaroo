package web

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/colormechadd/mailaroo/internal/outbound"
	"github.com/colormechadd/mailaroo/pkg/auth"
	"github.com/colormechadd/mailaroo/pkg/models"
	"github.com/colormechadd/mailaroo/templates"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleAdminRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// --- Users ---

func (s *Server) handleAdminUserList(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	users, err := s.AdminDB.ListUsers(r.Context())
	if err != nil {
		slog.Error("admin: failed to list users", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	mailboxes, err := s.AdminDB.ListAllMailboxes(r.Context())
	if err != nil {
		slog.Error("admin: failed to list mailboxes", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	dkimKeys, err := s.AdminDB.ListDKIMKeys(r.Context())
	if err != nil {
		slog.Error("admin: failed to list DKIM keys", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	domains := activeDomains(dkimKeys)
	s.renderAdmin(w, r, user, "users", templates.AdminUserList(users, mailboxes, domains, ""), "Users")
}

func activeDomains(keys []models.DKIMKey) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range keys {
		if k.IsActive && !seen[k.Domain] {
			seen[k.Domain] = true
			out = append(out, k.Domain)
		}
	}
	return out
}

func (s *Server) handleAdminUserCreate(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if username == "" || password == "" {
		http.Error(w, "Username and password required", http.StatusBadRequest)
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		slog.Error("admin: failed to hash password", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	recoveryEmail := strings.TrimSpace(r.FormValue("recovery_email"))
	u := &models.User{
		ID:            uuid.Must(uuid.NewV7()),
		Username:      username,
		PasswordHash:  hash,
		RecoveryEmail: recoveryEmail,
		IsActive:      true,
	}
	if err := s.AdminDB.CreateUserNoMailbox(r.Context(), u); err != nil {
		slog.Error("admin: failed to create user", "username", username, "error", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}
	// Clean up the user row if any subsequent step fails so we don't leave orphans.
	committed := false
	defer func() {
		if !committed {
			if err := s.AdminDB.DeleteUser(context.Background(), u.ID); err != nil {
				slog.Error("admin: failed to clean up orphan user", "user_id", u.ID, "error", err)
			}
		}
	}()

	mode := r.FormValue("mode")
	slog.Info("admin: create user form values", "mode", mode,
		"recv_local", r.FormValue("recv_local"), "recv_domain", r.FormValue("recv_domain"),
		"send_local", r.FormValue("send_local"), "send_domain", r.FormValue("send_domain"),
		"mailbox_name", r.FormValue("mailbox_name"),
	)
	if mode == "existing" {
		mbID, err := uuid.Parse(r.FormValue("mailbox_id"))
		if err != nil {
			http.Error(w, "Invalid mailbox ID", http.StatusBadRequest)
			return
		}
		if err := s.AdminDB.AddUserToMailbox(r.Context(), mbID, u.ID); err != nil {
			slog.Error("admin: failed to add user to mailbox", "error", err)
			http.Error(w, "Failed to assign mailbox", http.StatusInternalServerError)
			return
		}
		if local, domain := strings.TrimSpace(r.FormValue("sa_local")), strings.TrimSpace(r.FormValue("sa_domain")); local != "" && domain != "" {
			sa := &models.SendingAddress{ID: uuid.Must(uuid.NewV7()), UserID: u.ID, MailboxID: mbID, Address: local + "@" + domain, IsActive: true}
			if err := s.AdminDB.AddSendingAddress(r.Context(), sa); err != nil {
				slog.Error("admin: failed to add sending address", "error", err)
			}
		}
	} else {
		mbName := strings.TrimSpace(r.FormValue("mailbox_name"))
		if mbName == "" {
			mbName = username
		}
		mb := &models.Mailbox{ID: uuid.Must(uuid.NewV7()), Name: mbName}
		if err := s.AdminDB.CreateMailbox(r.Context(), mb, u.ID); err != nil {
			slog.Error("admin: failed to create mailbox", "error", err)
			http.Error(w, "Failed to create mailbox", http.StatusInternalServerError)
			return
		}
		recvLocal, recvDomain := strings.TrimSpace(r.FormValue("recv_local")), strings.TrimSpace(r.FormValue("recv_domain"))
		if recvLocal == "" {
			recvLocal = username
		}
		if recvDomain != "" {
			pattern := "^" + recvLocal + "@" + regexp.QuoteMeta(recvDomain) + "$"
			if _, err := regexp.Compile(pattern); err != nil {
				http.Error(w, "Invalid receive pattern: "+err.Error(), http.StatusBadRequest)
				return
			}
			am := &models.AddressMapping{
				ID:             uuid.Must(uuid.NewV7()),
				AddressPattern: pattern,
				MailboxID:      mb.ID,
				Priority:       10,
				IsActive:       true,
			}
			if err := s.AdminDB.CreateAddressMapping(r.Context(), am); err != nil {
				slog.Error("admin: failed to create address mapping", "error", err)
			}
		}
		sendLocal, sendDomain := strings.TrimSpace(r.FormValue("send_local")), strings.TrimSpace(r.FormValue("send_domain"))
		if sendLocal == "" {
			sendLocal = username
		}
		if sendDomain != "" {
			sa := &models.SendingAddress{ID: uuid.Must(uuid.NewV7()), UserID: u.ID, MailboxID: mb.ID, Address: sendLocal + "@" + sendDomain, IsActive: true}
			if err := s.AdminDB.AddSendingAddress(r.Context(), sa); err != nil {
				slog.Error("admin: failed to add sending address", "error", err)
			}
		}
	}

	committed = true
	s.redirectToUserDetail(w, r, u.ID)
}

func (s *Server) handleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value("user").(*models.User)
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	target, err := s.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	mailboxes, err := s.AdminDB.ListMailboxes(r.Context(), userID)
	if err != nil {
		slog.Error("admin: failed to list mailboxes for user", "user_id", userID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	allMailboxes, err := s.AdminDB.ListAllMailboxes(r.Context())
	if err != nil {
		slog.Error("admin: failed to list all mailboxes", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	addrs, err := s.AdminDB.ListSendingAddresses(r.Context(), userID)
	if err != nil {
		slog.Error("admin: failed to list sending addresses for user", "user_id", userID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	dkimKeys, err := s.AdminDB.ListDKIMKeys(r.Context())
	if err != nil {
		slog.Error("admin: failed to list DKIM keys", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	s.renderAdmin(w, r, admin, "users", templates.AdminUserDetail(*target, mailboxes, allMailboxes, addrs, activeDomains(dkimKeys), ""), target.Username)
}

func (s *Server) handleAdminUserToggleActive(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value("user").(*models.User)
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if userID == admin.ID {
		http.Error(w, "Cannot change your own active status", http.StatusBadRequest)
		return
	}
	if err := s.AdminDB.ToggleUserActive(r.Context(), userID); err != nil {
		slog.Error("admin: failed to toggle user active", "user_id", userID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	s.redirectToUserDetail(w, r, userID)
}

func (s *Server) handleAdminUserToggleAdmin(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value("user").(*models.User)
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if userID == admin.ID {
		http.Error(w, "Cannot change your own admin status", http.StatusBadRequest)
		return
	}
	if err := s.AdminDB.ToggleUserAdmin(r.Context(), userID); err != nil {
		slog.Error("admin: failed to toggle user admin", "user_id", userID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	s.redirectToUserDetail(w, r, userID)
}

func (s *Server) handleAdminUserSetRecoveryEmail(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("recovery_email"))
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			http.Error(w, "Invalid email address.", http.StatusBadRequest)
			return
		}
	}
	if err := s.AdminDB.UpdateUserRecoveryEmail(r.Context(), userID, email); err != nil {
		slog.Error("admin: failed to update recovery email", "user_id", userID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	s.redirectToUserDetail(w, r, userID)
}

func (s *Server) handleAdminUserAddSendingAddress(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	mailboxID, err := uuid.Parse(r.FormValue("mailbox_id"))
	if err != nil {
		http.Error(w, "Invalid mailbox ID", http.StatusBadRequest)
		return
	}
	local := strings.TrimSpace(r.FormValue("local"))
	domain := strings.TrimSpace(r.FormValue("domain"))
	if local == "" || domain == "" {
		http.Error(w, "Address and domain required", http.StatusBadRequest)
		return
	}
	address := local + "@" + domain
	sa := &models.SendingAddress{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		MailboxID: mailboxID,
		Address:   address,
		IsActive:  true,
	}
	if err := s.AdminDB.AddSendingAddress(r.Context(), sa); err != nil {
		slog.Error("admin: failed to add sending address", "address", address, "error", err)
		http.Error(w, "Failed to add sending address", http.StatusInternalServerError)
		return
	}
	s.redirectToUserDetail(w, r, userID)
}

func (s *Server) handleAdminUserAddMailbox(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if mbIDStr := r.FormValue("mailbox_id"); mbIDStr != "" {
		mbID, err := uuid.Parse(mbIDStr)
		if err != nil {
			http.Error(w, "Invalid mailbox ID", http.StatusBadRequest)
			return
		}
		if err := s.AdminDB.AddUserToMailbox(r.Context(), mbID, userID); err != nil {
			slog.Error("admin: failed to add user to mailbox", "user_id", userID, "mailbox_id", mbID, "error", err)
			http.Error(w, "Failed to add to mailbox", http.StatusInternalServerError)
			return
		}
	} else {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http.Error(w, "Name required", http.StatusBadRequest)
			return
		}
		mb := &models.Mailbox{ID: uuid.Must(uuid.NewV7()), Name: name}
		if err := s.AdminDB.CreateMailbox(r.Context(), mb, userID); err != nil {
			slog.Error("admin: failed to create mailbox for user", "user_id", userID, "error", err)
			http.Error(w, "Failed to create mailbox", http.StatusInternalServerError)
			return
		}
	}
	s.redirectToUserDetail(w, r, userID)
}

func (s *Server) redirectToUserDetail(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	dest := "/admin/users/" + userID.String()
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", dest)
		return
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// --- Mailboxes ---

func (s *Server) handleAdminMailboxList(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxes, err := s.AdminDB.ListAllMailboxes(r.Context())
	if err != nil {
		slog.Error("admin: failed to list mailboxes", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	s.renderAdmin(w, r, user, "mailboxes", templates.AdminMailboxList(mailboxes, ""), "Mailboxes")
}

func (s *Server) handleAdminMailboxCreate(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	name := strings.TrimSpace(r.FormValue("name"))
	if username == "" || name == "" {
		http.Error(w, "Username and mailbox name required", http.StatusBadRequest)
		return
	}
	u, err := s.AdminDB.GetUserByUsername(r.Context(), username)
	if err != nil {
		http.Error(w, "User not found", http.StatusBadRequest)
		return
	}
	mb := &models.Mailbox{ID: uuid.Must(uuid.NewV7()), Name: name}
	if err := s.AdminDB.CreateMailbox(r.Context(), mb, u.ID); err != nil {
		slog.Error("admin: failed to create mailbox", "name", name, "error", err)
		http.Error(w, "Failed to create mailbox", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/admin/mailboxes")
		return
	}
	http.Redirect(w, r, "/admin/mailboxes", http.StatusSeeOther)
}

func (s *Server) handleAdminMailboxDetail(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxID, err := uuid.Parse(chi.URLParam(r, "mailboxID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	mb, err := s.AdminDB.GetMailboxByID(r.Context(), mailboxID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	mbUsers, err := s.DB.GetMailboxUsers(r.Context(), mailboxID)
	if err != nil {
		slog.Error("admin: failed to get mailbox users", "mailbox_id", mailboxID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	mappings, err := s.AdminDB.GetAddressMappingsByMailboxID(r.Context(), mailboxID)
	if err != nil {
		slog.Error("admin: failed to get address mappings", "mailbox_id", mailboxID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	sendingAddrs, err := s.AdminDB.ListSendingAddressesByMailboxID(r.Context(), mailboxID)
	if err != nil {
		slog.Error("admin: failed to list sending addresses for mailbox", "mailbox_id", mailboxID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	allUsers, err := s.AdminDB.ListUsers(r.Context())
	if err != nil {
		slog.Error("admin: failed to list users", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	dkimKeys, err := s.AdminDB.ListDKIMKeys(r.Context())
	if err != nil {
		slog.Error("admin: failed to list DKIM keys", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	from := safeRedirect(r.URL.Query().Get("from"), "/admin/mailboxes")
	s.renderAdmin(w, r, user, "mailboxes", templates.AdminMailboxDetail(*mb, mbUsers, allUsers, mappings, sendingAddrs, activeDomains(dkimKeys), from, ""), mb.Name)
}

func (s *Server) handleAdminMailboxRemoveUser(w http.ResponseWriter, r *http.Request) {
	mailboxID, err := uuid.Parse(chi.URLParam(r, "mailboxID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}
	if err := s.AdminDB.RemoveUserFromMailbox(r.Context(), mailboxID, userID); err != nil {
		slog.Error("admin: failed to remove user from mailbox", "mailbox_id", mailboxID, "user_id", userID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	dest := fmt.Sprintf("/admin/mailboxes/%s", mailboxID)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", dest)
		return
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (s *Server) handleAdminMailboxAddUser(w http.ResponseWriter, r *http.Request) {
	mailboxID, err := uuid.Parse(chi.URLParam(r, "mailboxID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	userID, err := uuid.Parse(r.FormValue("user_id"))
	if err != nil {
		http.Error(w, "Invalid user", http.StatusBadRequest)
		return
	}
	if err := s.AdminDB.AddUserToMailbox(r.Context(), mailboxID, userID); err != nil {
		slog.Error("admin: failed to add user to mailbox", "mailbox_id", mailboxID, "user_id", userID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	redirect := fmt.Sprintf("/admin/mailboxes/%s", mailboxID)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirect)
		return
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) handleAdminMailboxAddMapping(w http.ResponseWriter, r *http.Request) {
	mailboxID, err := uuid.Parse(chi.URLParam(r, "mailboxID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	local := strings.TrimSpace(r.FormValue("local"))
	domain := strings.TrimSpace(r.FormValue("domain"))
	if local == "" || domain == "" {
		http.Error(w, "Local part and domain required", http.StatusBadRequest)
		return
	}
	pattern := "^" + regexp.QuoteMeta(local) + "@" + regexp.QuoteMeta(domain) + "$"
	am := &models.AddressMapping{
		ID:             uuid.Must(uuid.NewV7()),
		AddressPattern: pattern,
		MailboxID:      mailboxID,
		Priority:       10,
		IsActive:       true,
	}
	if err := s.AdminDB.CreateAddressMapping(r.Context(), am); err != nil {
		slog.Error("admin: failed to create address mapping", "pattern", pattern, "error", err)
		http.Error(w, "Failed to create mapping", http.StatusInternalServerError)
		return
	}
	redirect := fmt.Sprintf("/admin/mailboxes/%s", mailboxID)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirect)
		return
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// --- Sending Addresses ---

func (s *Server) handleAdminSendingAddressList(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	addrs, err := s.AdminDB.ListAllSendingAddresses(r.Context())
	if err != nil {
		slog.Error("admin: failed to list sending addresses", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	users, err := s.AdminDB.ListUsers(r.Context())
	if err != nil {
		slog.Error("admin: failed to list users", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	mailboxes, err := s.AdminDB.ListAllMailboxes(r.Context())
	if err != nil {
		slog.Error("admin: failed to list mailboxes", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	s.renderAdmin(w, r, user, "sending-addresses", templates.AdminSendingAddressList(addrs, users, mailboxes, ""), "Sending Addresses")
}

func (s *Server) handleAdminSendingAddressCreate(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.FormValue("user_id")
	mailboxIDStr := r.FormValue("mailbox_id")
	address := strings.TrimSpace(r.FormValue("address"))
	if userIDStr == "" || mailboxIDStr == "" || address == "" {
		http.Error(w, "All fields required", http.StatusBadRequest)
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}
	mailboxID, err := uuid.Parse(mailboxIDStr)
	if err != nil {
		http.Error(w, "Invalid mailbox ID", http.StatusBadRequest)
		return
	}
	sa := &models.SendingAddress{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		MailboxID: mailboxID,
		Address:   address,
		IsActive:  true,
	}
	if err := s.AdminDB.AddSendingAddress(r.Context(), sa); err != nil {
		slog.Error("admin: failed to add sending address", "address", address, "error", err)
		http.Error(w, "Failed to add sending address", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/admin/sending-addresses")
		return
	}
	http.Redirect(w, r, "/admin/sending-addresses", http.StatusSeeOther)
}

func (s *Server) handleAdminSendingAddressReactivate(w http.ResponseWriter, r *http.Request) {
	saID, err := uuid.Parse(chi.URLParam(r, "saID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if err := s.AdminDB.ReactivateSendingAddress(r.Context(), saID); err != nil {
		slog.Error("admin: failed to reactivate sending address", "sa_id", saID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	dest := safeRedirect(r.FormValue("redirect_to"), "/admin/sending-addresses")
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", dest)
		return
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (s *Server) handleAdminSendingAddressDeactivate(w http.ResponseWriter, r *http.Request) {
	saID, err := uuid.Parse(chi.URLParam(r, "saID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if err := s.AdminDB.DeactivateSendingAddress(r.Context(), saID); err != nil {
		slog.Error("admin: failed to deactivate sending address", "sa_id", saID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	dest := safeRedirect(r.FormValue("redirect_to"), "/admin/sending-addresses")
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", dest)
		return
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// --- Domains / DKIM ---

func (s *Server) handleAdminDomainList(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	keys, err := s.AdminDB.ListDKIMKeys(r.Context())
	if err != nil {
		slog.Error("admin: failed to list DKIM keys", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	s.renderAdmin(w, r, user, "domains", templates.AdminDomainList(keys, ""), "Domains")
}

func (s *Server) handleAdminDomainCreate(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	if len(s.DKIMEncKey) == 0 {
		http.Error(w, "DKIM encryption key not configured", http.StatusInternalServerError)
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.FormValue("domain")))
	if domain == "" {
		http.Error(w, "Domain required", http.StatusBadRequest)
		return
	}
	der, err := outbound.GenerateKeyDER()
	if err != nil {
		slog.Error("admin: failed to generate DKIM key", "domain", domain, "error", err)
		http.Error(w, "Failed to generate key", http.StatusInternalServerError)
		return
	}
	encrypted, err := outbound.EncryptKey(der, s.DKIMEncKey)
	if err != nil {
		slog.Error("admin: failed to encrypt DKIM key", "error", err)
		http.Error(w, "Failed to encrypt key", http.StatusInternalServerError)
		return
	}
	key := &models.DKIMKey{
		ID:       uuid.Must(uuid.NewV7()),
		Domain:   domain,
		Selector: "mailaroo",
		KeyData:  encrypted,
		IsActive: true,
	}
	if err := s.AdminDB.InsertDKIMKey(r.Context(), key); err != nil {
		slog.Error("admin: failed to insert DKIM key", "domain", domain, "error", err)
		http.Error(w, "Failed to save key", http.StatusInternalServerError)
		return
	}
	dnsValue, err := outbound.DERToDNSValue(der)
	if err != nil {
		slog.Error("admin: failed to derive DNS value", "error", err)
		http.Error(w, "Failed to derive DNS value", http.StatusInternalServerError)
		return
	}
	smtpDomain := s.Config.SMTP.Domain
	mxRecord := smtpDomain + "."
	spfRecord := fmt.Sprintf("v=spf1 mx include:%s ~all", smtpDomain)
	s.renderAdmin(w, r, user, "domains", templates.AdminDomainDNS(domain, key.Selector, mxRecord, spfRecord, dnsValue, "", nil), domain+" DNS")
}

func (s *Server) handleAdminDomainDNS(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	if len(s.DKIMEncKey) == 0 {
		http.Error(w, "DKIM encryption key not configured", http.StatusInternalServerError)
		return
	}
	domain := chi.URLParam(r, "domain")
	key, err := s.AdminDB.GetActiveDKIMKey(r.Context(), domain, nil)
	if err != nil {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	}
	der, err := outbound.DecryptKey(key.KeyData, s.DKIMEncKey)
	if err != nil {
		slog.Error("admin: failed to decrypt DKIM key", "domain", domain, "error", err)
		http.Error(w, "Failed to read key", http.StatusInternalServerError)
		return
	}
	dnsValue, err := outbound.DERToDNSValue(der)
	if err != nil {
		slog.Error("admin: failed to derive DNS value", "error", err)
		http.Error(w, "Failed to derive DNS value", http.StatusInternalServerError)
		return
	}
	smtpDomain := s.Config.SMTP.Domain
	mxRecord := smtpDomain + "."
	spfRecord := fmt.Sprintf("v=spf1 mx include:%s ~all", smtpDomain)
	s.renderAdmin(w, r, user, "domains", templates.AdminDomainDNS(domain, key.Selector, mxRecord, spfRecord, dnsValue, "", nil), domain+" DNS")
}

func (s *Server) handleAdminDomainVerify(w http.ResponseWriter, r *http.Request) {
	if len(s.DKIMEncKey) == 0 {
		http.Error(w, "DKIM encryption key not configured", http.StatusInternalServerError)
		return
	}
	domain := chi.URLParam(r, "domain")
	key, err := s.AdminDB.GetActiveDKIMKey(r.Context(), domain, nil)
	if err != nil {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	}
	der, err := outbound.DecryptKey(key.KeyData, s.DKIMEncKey)
	if err != nil {
		http.Error(w, "Failed to read key", http.StatusInternalServerError)
		return
	}
	expectedDKIM, err := outbound.DERToDNSValue(der)
	if err != nil {
		http.Error(w, "Failed to derive DNS value", http.StatusInternalServerError)
		return
	}
	smtpDomain := s.Config.SMTP.Domain

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	mx := checkMX(ctx, domain, smtpDomain)
	spf := checkSPF(ctx, domain, smtpDomain)
	dkim := checkDKIM(ctx, domain, key.Selector, expectedDKIM)

	results := &[3]templates.DNSCheckResult{mx, spf, dkim}
	user := r.Context().Value("user").(*models.User)
	mxRecord := smtpDomain + "."
	spfRecord := fmt.Sprintf("v=spf1 mx include:%s ~all", smtpDomain)
	s.renderAdmin(w, r, user, "domains", templates.AdminDomainDNS(domain, key.Selector, mxRecord, spfRecord, expectedDKIM, "", results), domain+" DNS")
}

func (s *Server) handleAdminDomainRotate(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	if len(s.DKIMEncKey) == 0 {
		http.Error(w, "DKIM encryption key not configured", http.StatusInternalServerError)
		return
	}
	domain := chi.URLParam(r, "domain")
	existing, err := s.AdminDB.GetActiveDKIMKey(r.Context(), domain, nil)
	if err != nil {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	}
	der, err := outbound.GenerateKeyDER()
	if err != nil {
		slog.Error("admin: failed to generate new DKIM key", "domain", domain, "error", err)
		http.Error(w, "Failed to generate key", http.StatusInternalServerError)
		return
	}
	encrypted, err := outbound.EncryptKey(der, s.DKIMEncKey)
	if err != nil {
		http.Error(w, "Failed to encrypt key", http.StatusInternalServerError)
		return
	}
	if err := s.AdminDB.UpdateDKIMKeyData(r.Context(), existing.ID, encrypted); err != nil {
		slog.Error("admin: failed to update DKIM key", "domain", domain, "error", err)
		http.Error(w, "Failed to update key", http.StatusInternalServerError)
		return
	}
	dnsValue, err := outbound.DERToDNSValue(der)
	if err != nil {
		http.Error(w, "Failed to derive DNS value", http.StatusInternalServerError)
		return
	}
	smtpDomain := s.Config.SMTP.Domain
	mxRecord := smtpDomain + "."
	spfRecord := fmt.Sprintf("v=spf1 mx include:%s ~all", smtpDomain)
	s.renderAdmin(w, r, user, "domains", templates.AdminDomainDNS(domain, existing.Selector, mxRecord, spfRecord, dnsValue, "", nil), domain+" DNS")
}

func checkMX(ctx context.Context, domain string, smtpDomain string) templates.DNSCheckResult {
	resolver := net.DefaultResolver
	mxRecords, err := resolver.LookupMX(ctx, domain)
	if err != nil || len(mxRecords) == 0 {
		return templates.DNSCheckResult{Expected: smtpDomain}
	}
	for _, mx := range mxRecords {
		host := strings.TrimSuffix(mx.Host, ".")
		if host == smtpDomain || strings.HasSuffix(host, "."+smtpDomain) {
			return templates.DNSCheckResult{Passed: true, Found: fmt.Sprintf("MX %d %s", mx.Pref, mx.Host)}
		}
	}
	found := fmt.Sprintf("%s (priority %d)", mxRecords[0].Host, mxRecords[0].Pref)
	return templates.DNSCheckResult{Found: found, Expected: smtpDomain}
}

func checkSPF(ctx context.Context, domain string, smtpDomain string) templates.DNSCheckResult {
	expected := fmt.Sprintf("v=spf1 mx include:%s ~all", smtpDomain)
	resolver := net.DefaultResolver

	serverIPs, _ := resolver.LookupHost(ctx, smtpDomain)

	txts, err := resolver.LookupTXT(ctx, domain)
	if err != nil {
		return templates.DNSCheckResult{Expected: expected}
	}
	var spfRecord string
	for _, txt := range txts {
		if strings.HasPrefix(txt, "v=spf1") {
			spfRecord = txt
			break
		}
	}
	if spfRecord == "" {
		return templates.DNSCheckResult{Expected: expected}
	}
	if len(serverIPs) > 0 && spfAuthorizes(ctx, resolver, domain, spfRecord, serverIPs, 0) {
		return templates.DNSCheckResult{Passed: true, Found: spfRecord}
	}
	return templates.DNSCheckResult{Found: spfRecord, Expected: expected}
}

func checkDKIM(ctx context.Context, domain string, selector string, expectedValue string) templates.DNSCheckResult {
	dkimHost := selector + "._domainkey." + domain
	resolver := net.DefaultResolver
	txts, err := resolver.LookupTXT(ctx, dkimHost)
	if err != nil || len(txts) == 0 {
		return templates.DNSCheckResult{Expected: expectedValue}
	}
	full := strings.Join(txts, "")
	if strings.Contains(full, "v=DKIM1") {
		pubKeyStart := strings.Index(expectedValue, "p=")
		foundStart := strings.Index(full, "p=")
		if pubKeyStart >= 0 && foundStart >= 0 {
			expectedPub := expectedValue[pubKeyStart:]
			foundPub := full[foundStart:]
			if expectedPub == foundPub {
				return templates.DNSCheckResult{Passed: true, Found: truncateDKIM(full)}
			}
		}
		return templates.DNSCheckResult{Found: truncateDKIM(full), Expected: truncateDKIM(expectedValue)}
	}
	return templates.DNSCheckResult{Found: full, Expected: expectedValue}
}

func truncateDKIM(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}
