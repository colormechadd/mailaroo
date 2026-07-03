package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/colormechadd/mailaroo/internal/config"
	"github.com/colormechadd/mailaroo/internal/db"
	"github.com/colormechadd/mailaroo/internal/outbound"
	"github.com/colormechadd/mailaroo/pkg/auth"
	"github.com/colormechadd/mailaroo/pkg/models"
	"github.com/google/uuid"
)

func bootstrapFromConfig(ctx context.Context, database *db.DB, path string, dkimEncKey []byte) error {
	bootCfg, err := config.LoadBootstrapConfig(path)
	if err != nil {
		return fmt.Errorf("loading bootstrap config: %w", err)
	}

	userIDs := make(map[string]uuid.UUID, len(bootCfg.Users))

	for _, u := range bootCfg.Users {
		existing, err := database.GetUserByUsername(ctx, u.Username)
		if err == nil {
			slog.Info("user already exists, skipping", "username", u.Username)
			userIDs[u.Username] = existing.ID
			continue
		}

		hash, err := auth.HashPassword(u.Password)
		if err != nil {
			return fmt.Errorf("hashing password for %s: %w", u.Username, err)
		}

		user := &models.User{
			ID:            uuid.New(),
			Username:      u.Username,
			PasswordHash:  hash,
			RecoveryEmail: u.RecoveryEmail,
			IsActive:      true,
			IsAdmin:       u.IsAdmin,
		}

		if err := database.CreateUserNoMailbox(ctx, user); err != nil {
			return fmt.Errorf("creating user %s: %w", u.Username, err)
		}
		userIDs[u.Username] = user.ID
		slog.Info("created user from bootstrap config", "username", u.Username)
	}

	for _, mb := range bootCfg.Mailboxes {
		if len(mb.Users) == 0 {
			slog.Warn("mailbox has no users, skipping", "mailbox", mb.Name)
			continue
		}

		ownerID, ok := userIDs[mb.Users[0]]
		if !ok {
			return fmt.Errorf("mailbox %q references unknown user %q", mb.Name, mb.Users[0])
		}

		mailbox := &models.Mailbox{
			ID:   uuid.New(),
			Name: mb.Name,
		}
		if err := database.CreateMailbox(ctx, mailbox, ownerID); err != nil {
			return fmt.Errorf("creating mailbox %q: %w", mb.Name, err)
		}

		for _, username := range mb.Users[1:] {
			uid, ok := userIDs[username]
			if !ok {
				return fmt.Errorf("mailbox %q references unknown user %q", mb.Name, username)
			}
			if err := database.AddUserToMailbox(ctx, mailbox.ID, uid); err != nil {
				return fmt.Errorf("adding user %q to mailbox %q: %w", username, mb.Name, err)
			}
		}

		for _, am := range mb.AddressMappings {
			mapping := &models.AddressMapping{
				ID:             uuid.New(),
				AddressPattern: am.Pattern,
				MailboxID:      mailbox.ID,
				Priority:       am.Priority,
				IsActive:       true,
			}
			if err := database.CreateAddressMapping(ctx, mapping); err != nil {
				return fmt.Errorf("creating address mapping %q: %w", am.Pattern, err)
			}
		}

		for _, sa := range mb.SendingAddresses {
			if sa.User == "" {
				return fmt.Errorf("sending address %q in mailbox %q has no user", sa.Address, mb.Name)
			}
			uid, ok := userIDs[sa.User]
			if !ok {
				return fmt.Errorf("sending address %q references unknown user %q", sa.Address, sa.User)
			}
			addr := &models.SendingAddress{
				ID:        uuid.New(),
				UserID:    uid,
				MailboxID: mailbox.ID,
				Address:   sa.Address,
				IsActive:  true,
			}
			if sa.DisplayName != "" {
				addr.DisplayName = &sa.DisplayName
			}
			if err := database.AddSendingAddress(ctx, addr); err != nil {
				return fmt.Errorf("creating sending address %q: %w", sa.Address, err)
			}
		}
	}

	for _, d := range bootCfg.Domains {
		existing, err := database.GetActiveDKIMKey(ctx, d.Domain, nil)
		if err == nil && existing != nil {
			slog.Info("DKIM key already exists for domain, skipping", "domain", d.Domain)
			continue
		}

		der, err := outbound.GenerateKeyDER()
		if err != nil {
			return fmt.Errorf("generating DKIM key for %s: %w", d.Domain, err)
		}

		keyData := der
		if dkimEncKey != nil {
			encrypted, err := outbound.EncryptKey(der, dkimEncKey)
			if err != nil {
				return fmt.Errorf("encrypting DKIM key for %s: %w", d.Domain, err)
			}
			keyData = encrypted
		}

		selector := d.Selector
		if selector == "" {
			selector = "mailaroo"
		}

		key := &models.DKIMKey{
			ID:       uuid.Must(uuid.NewV7()),
			Domain:   d.Domain,
			Selector: selector,
			KeyData:  keyData,
			IsActive: true,
		}
		if err := database.InsertDKIMKey(ctx, key); err != nil {
			return fmt.Errorf("inserting DKIM key for %s: %w", d.Domain, err)
		}
		slog.Info("created DKIM key from bootstrap config", "domain", d.Domain, "selector", selector)
	}

	return nil
}
