package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"github.com/colormechadd/mailaroo/internal/db"
	"github.com/colormechadd/mailaroo/pkg/models"
	"github.com/colormechadd/mailaroo/templates"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) contactBlockRule(ctx context.Context, mailboxID uuid.UUID, c *models.Contact) *models.MailboxBlockRule {
	if c == nil {
		return nil
	}
	rule, _ := s.DB.IsBlockedByMailboxRules(ctx, mailboxID, c.Email)
	return rule
}

func mailboxName(mailboxes []models.Mailbox, mailboxID uuid.UUID) string {
	for _, mb := range mailboxes {
		if mb.ID == mailboxID {
			return mb.Name
		}
	}
	return ""
}

func (s *Server) handleContactsPage(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxID, mailboxes, err := s.getMailboxForUser(r, chi.URLParam(r, "mailboxID"), user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	contacts, err := s.DB.ListContacts(r.Context(), mailboxID)
	if err != nil {
		s.logger.Error("failed to list contacts", "mailbox_id", mailboxID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	selectedIDRaw := r.URL.Query().Get("id")
	var selected *models.Contact
	if selectedIDRaw != "" {
		if id, err := uuid.Parse(selectedIDRaw); err == nil {
			for i := range contacts {
				if contacts[i].ID == id {
					selected = &contacts[i]
					break
				}
			}
		}
	}

	var recentEmails []models.Email
	if selected != nil {
		recentEmails, err = s.DB.SearchEmails(r.Context(), mailboxID, user.ID, db.EmailFilter{Participant:selected.Email}, 3, nil, nil)
		if err != nil {
			s.logger.Error("failed to fetch recent emails for contact", "contact_id", selected.ID, "error", err)
		}
	}

	showNew := r.URL.Query().Get("new") == "1"
	counts := s.getCounts(r.Context(), mailboxID, user.ID)
	s.render(w, r, user, mailboxes, mailboxID, "contacts", counts, templates.ContactsPage(mailboxID, mailboxName(mailboxes, mailboxID), contacts, selected, recentEmails, showNew, s.contactBlockRule(r.Context(), mailboxID, selected)), "Contacts")
}

func (s *Server) handleContactView(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxID, mailboxes, err := s.getMailboxForUser(r, chi.URLParam(r, "mailboxID"), user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	contactID, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	contacts, err := s.DB.ListContacts(r.Context(), mailboxID)
	if err != nil {
		s.logger.Error("failed to list contacts", "mailbox_id", mailboxID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	var selected *models.Contact
	for i := range contacts {
		if contacts[i].ID == contactID {
			selected = &contacts[i]
			break
		}
	}
	if selected == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	recentEmails, err := s.DB.SearchEmails(r.Context(), mailboxID, user.ID, db.EmailFilter{Participant:selected.Email}, 3, nil, nil)
	if err != nil {
		s.logger.Error("failed to fetch recent emails for contact", "contact_id", contactID, "error", err)
	}

	counts := s.getCounts(r.Context(), mailboxID, user.ID)
	s.render(w, r, user, mailboxes, mailboxID, "contacts", counts, templates.ContactsPage(mailboxID, mailboxName(mailboxes, mailboxID), contacts, selected, recentEmails, false, s.contactBlockRule(r.Context(), mailboxID, selected)), "Contacts")
}

func (s *Server) handleContactSearch(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	contacts, err := s.DB.SearchContactsForUser(r.Context(), user.ID, q)
	if err != nil {
		s.logger.Error("failed to search contacts", "user_id", user.ID, "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	type result struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Initials string `json:"initials"`
		Display  string `json:"display"`
	}
	out := make([]result, 0, len(contacts))
	for _, c := range contacts {
		display := c.Email
		name := strings.TrimSpace(c.FirstName + " " + c.LastName)
		if name != "" {
			display = name + " <" + c.Email + ">"
		}
		out = append(out, result{
			ID:       c.ID.String(),
			Name:     name,
			Email:    c.Email,
			Initials: c.Initials(),
			Display:  display,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) handleContactCreate(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxID, _, err := s.getMailboxForUser(r, chi.URLParam(r, "mailboxID"), user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	c := models.Contact{
		MailboxID:  mailboxID,
		FirstName:  strings.TrimSpace(r.FormValue("first_name")),
		LastName:   strings.TrimSpace(r.FormValue("last_name")),
		Email:      strings.TrimSpace(r.FormValue("email")),
		Phone:      strings.TrimSpace(r.FormValue("phone")),
		Street:     strings.TrimSpace(r.FormValue("street")),
		City:       strings.TrimSpace(r.FormValue("city")),
		State:      strings.TrimSpace(r.FormValue("state")),
		PostalCode: strings.TrimSpace(r.FormValue("postal_code")),
		Country:    strings.TrimSpace(r.FormValue("country")),
		Notes:      strings.TrimSpace(r.FormValue("notes")),
		CustomCSS:  strings.TrimSpace(r.FormValue("custom_css")),
		IsFavorite: r.FormValue("is_favorite") == "true" || r.FormValue("is_favorite") == "on",
	}

	if c.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}
	if err := validateCSS(c.CustomCSS); err != nil {
		http.Error(w, "Invalid CSS: "+err.Error(), http.StatusBadRequest)
		return
	}

	created, err := s.DB.CreateContact(r.Context(), c)
	if err != nil {
		s.logger.Error("failed to create contact", "mailbox_id", mailboxID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	contacts, _ := s.DB.ListContacts(r.Context(), mailboxID)
	recentEmails, _ := s.DB.SearchEmails(r.Context(), mailboxID, user.ID, db.EmailFilter{Participant: created.Email}, 3, nil, nil)
	mailboxes, _ := s.DB.GetMailboxesByUserID(r.Context(), user.ID)
	templates.ContactsPage(mailboxID, mailboxName(mailboxes, mailboxID), contacts, created, recentEmails, false, s.contactBlockRule(r.Context(), mailboxID, created)).Render(r.Context(), w)
}

func (s *Server) handleContactUpdate(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxID, _, err := s.getMailboxForUser(r, chi.URLParam(r, "mailboxID"), user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	contactID, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	c := models.Contact{
		ID:         contactID,
		MailboxID:  mailboxID,
		FirstName:  strings.TrimSpace(r.FormValue("first_name")),
		LastName:   strings.TrimSpace(r.FormValue("last_name")),
		Email:      strings.TrimSpace(r.FormValue("email")),
		Phone:      strings.TrimSpace(r.FormValue("phone")),
		Street:     strings.TrimSpace(r.FormValue("street")),
		City:       strings.TrimSpace(r.FormValue("city")),
		State:      strings.TrimSpace(r.FormValue("state")),
		PostalCode: strings.TrimSpace(r.FormValue("postal_code")),
		Country:    strings.TrimSpace(r.FormValue("country")),
		Notes:      strings.TrimSpace(r.FormValue("notes")),
		CustomCSS:  strings.TrimSpace(r.FormValue("custom_css")),
		IsFavorite: r.FormValue("is_favorite") == "true" || r.FormValue("is_favorite") == "on",
	}

	if c.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}
	if err := validateCSS(c.CustomCSS); err != nil {
		http.Error(w, "Invalid CSS: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.DB.UpdateContact(r.Context(), c); err != nil {
		s.logger.Error("failed to update contact", "contact_id", contactID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	updated, _ := s.DB.GetContactByID(r.Context(), contactID, mailboxID)
	contacts, _ := s.DB.ListContacts(r.Context(), mailboxID)
	var recentEmails []models.Email
	if updated != nil {
		recentEmails, _ = s.DB.SearchEmails(r.Context(), mailboxID, user.ID, db.EmailFilter{Participant: updated.Email}, 3, nil, nil)
	}
	mailboxes, _ := s.DB.GetMailboxesByUserID(r.Context(), user.ID)
	templates.ContactsPage(mailboxID, mailboxName(mailboxes, mailboxID), contacts, updated, recentEmails, false, s.contactBlockRule(r.Context(), mailboxID, updated)).Render(r.Context(), w)
}

func (s *Server) handleContactDelete(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxID, _, err := s.getMailboxForUser(r, chi.URLParam(r, "mailboxID"), user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	contactID, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.DB.DeleteContact(r.Context(), contactID, mailboxID); err != nil {
		s.logger.Error("failed to delete contact", "contact_id", contactID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	contacts, _ := s.DB.ListContacts(r.Context(), mailboxID)
	mailboxes, _ := s.DB.GetMailboxesByUserID(r.Context(), user.ID)
	templates.ContactsPage(mailboxID, mailboxName(mailboxes, mailboxID), contacts, nil, nil, false, nil).Render(r.Context(), w)
}

func (s *Server) handleContactToggleFavorite(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxID, _, err := s.getMailboxForUser(r, chi.URLParam(r, "mailboxID"), user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	contactID, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.DB.ToggleContactFavorite(r.Context(), contactID, mailboxID); err != nil {
		s.logger.Error("failed to toggle favorite", "contact_id", contactID, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	updated, _ := s.DB.GetContactByID(r.Context(), contactID, mailboxID)
	contacts, _ := s.DB.ListContacts(r.Context(), mailboxID)
	var recentEmails []models.Email
	if updated != nil {
		recentEmails, _ = s.DB.SearchEmails(r.Context(), mailboxID, user.ID, db.EmailFilter{Participant: updated.Email}, 3, nil, nil)
	}
	mailboxes, _ := s.DB.GetMailboxesByUserID(r.Context(), user.ID)
	templates.ContactsPage(mailboxID, mailboxName(mailboxes, mailboxID), contacts, updated, recentEmails, false, s.contactBlockRule(r.Context(), mailboxID, updated)).Render(r.Context(), w)
}

func (s *Server) handleContactToggleBlock(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	mailboxID, _, err := s.getMailboxForUser(r, chi.URLParam(r, "mailboxID"), user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	contactID, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	contact, err := s.DB.GetContactByID(r.Context(), contactID, mailboxID)
	if err != nil || contact == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	rule, _ := s.DB.IsBlockedByMailboxRules(r.Context(), mailboxID, contact.Email)
	if rule != nil {
		if err := s.DB.DeleteBlockRule(r.Context(), rule.ID, mailboxID); err != nil {
			s.logger.Error("failed to delete block rule", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	} else {
		pattern := regexp.QuoteMeta(contact.Email)
		if err := s.DB.CreateBlockRule(r.Context(), mailboxID, user.ID, pattern); err != nil {
			s.logger.Error("failed to create block rule", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	}

	contacts, _ := s.DB.ListContacts(r.Context(), mailboxID)
	recentEmails, _ := s.DB.SearchEmails(r.Context(), mailboxID, user.ID, db.EmailFilter{Participant: contact.Email}, 3, nil, nil)
	mailboxes, _ := s.DB.GetMailboxesByUserID(r.Context(), user.ID)
	newRule, _ := s.DB.IsBlockedByMailboxRules(r.Context(), mailboxID, contact.Email)
	templates.ContactsPage(mailboxID, mailboxName(mailboxes, mailboxID), contacts, contact, recentEmails, false, newRule).Render(r.Context(), w)
}

func (s *Server) handleAddContactFromEmail(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*models.User)
	emailID, err := uuid.Parse(chi.URLParam(r, "emailID"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	email, err := s.DB.GetEmailByIDForUser(r.Context(), emailID, user.ID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	addr, err := mail.ParseAddress(email.FromAddress)
	firstName := ""
	lastName := ""
	emailAddr := email.FromAddress
	if err == nil {
		emailAddr = addr.Address
		parts := strings.Fields(addr.Name)
		if len(parts) == 1 {
			firstName = parts[0]
		} else if len(parts) >= 2 {
			firstName = strings.Join(parts[:len(parts)-1], " ")
			lastName = parts[len(parts)-1]
		}
	}

	if err := s.DB.UpsertContactFromEmail(r.Context(), email.MailboxID, emailAddr, firstName, lastName); err != nil {
		s.logger.Error("failed to upsert contact from email", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	contact, _ := s.DB.GetContactByEmail(r.Context(), email.MailboxID, emailAddr)
	templates.EmailSenderInfo(email, contact, false).Render(r.Context(), w)
}
