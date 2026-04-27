# Plan: User-Defined Email Filters

## Goal

Let users define rules that match incoming emails on conditions (sender, subject, body keywords, has attachment) and apply actions (archive, delete, move to folder, mark read, star, quarantine).

---

## Data Model

### New table: `mailbox_filter_rule`

```sql
CREATE TABLE mailbox_filter_rule (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    priority INT NOT NULL DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    match_all BOOLEAN DEFAULT TRUE,  -- true=AND, false=OR across conditions
    action TEXT NOT NULL,            -- see Actions below
    stop_processing BOOLEAN DEFAULT TRUE,  -- stop evaluating further rules on match
    created_by_user_id UUID REFERENCES "user"(id) ON DELETE SET NULL,
    updated_by_user_id UUID REFERENCES "user"(id) ON DELETE SET NULL,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE mailbox_filter_condition (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    rule_id UUID NOT NULL REFERENCES mailbox_filter_rule(id) ON DELETE CASCADE,
    field TEXT NOT NULL,    -- 'from', 'to', 'subject', 'body', 'has_attachment'
    operator TEXT NOT NULL, -- 'contains', 'not_contains', 'matches_regex', 'is', 'is_not'
    value TEXT,             -- null for has_attachment
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

**Actions:** `archive`, `delete`, `mark_read`, `star`, `quarantine`

**Notes:**
- Rules are ordered by `priority ASC` — lower number runs first.
- `stop_processing=true` (default) mimics Gmail-style "stop here" behavior.
- Reuses the existing `email_status` enum for the resulting status where applicable.

---

## Rule Engine

A dedicated `internal/filterengine` package encapsulates all rule evaluation logic, keeping it independent of the pipeline and reusable (e.g. for retroactive application or testing rules in the UI).

```go
// RuleEngine evaluates a set of filter rules against an email message.
type RuleEngine struct{}

// Match returns the first matching rule and its action, or nil if none match.
func (e *RuleEngine) Match(rules []*models.FilterRule, msg *ParsedMessage) (*models.FilterRule, error)

// ParsedMessage holds the fields the engine evaluates against.
type ParsedMessage struct {
    From          string
    To            string
    Subject       string
    Body          string
    HasAttachment bool
}
```

Condition evaluation (`contains`, `not_contains`, `matches_regex`, `is`, `is_not`) lives entirely inside this package. The engine respects `match_all` (AND) vs OR across conditions and `stop_processing` across rules.

---

## Pipeline Step

Add `step_apply_filter_rules.go` between `check_spam`/`block` and `finalize`:

```
... → check_spam → block → apply_filter_rules → finalize → notify
```

The step:
1. Loads active rules for `ictx.TargetMailboxID` from the DB, ordered by priority.
2. Calls `RuleEngine.Match` with a `ParsedMessage` built from the ingestion context.
3. On match, records the matched rule ID + action in `IngestionContext`, then returns.
4. Returns `StatusPass` always — actions are applied in `Finalize`, not as a rejection.

`IngestionContext` gets two new fields:
```go
MatchedFilterRuleID *uuid.UUID
FilterAction        string
```

`Finalize` reads these to set the initial `email.status` and `email.is_read`/`email.is_star` accordingly.

---

## DB Layer

New methods on `PipelineDB` interface:

```go
GetActiveFilterRulesForMailbox(ctx, mailboxID) ([]*models.FilterRule, error)
// Returns rules with conditions eagerly loaded, ordered by priority.
```

New CRUD methods on `WebDB` interface (for the UI):

```go
ListFilterRules(ctx, mailboxID) ([]*models.FilterRule, error)
CreateFilterRule(ctx, rule *models.FilterRule) error
UpdateFilterRule(ctx, rule *models.FilterRule) error
DeleteFilterRule(ctx, ruleID) error
ReorderFilterRules(ctx, mailboxID, orderedIDs []uuid.UUID) error
```

---

## Frontend (HTMX + Templ)

New routes under the mailbox settings section:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/mailbox/{mailboxID}/filters` | List rules page |
| GET | `/mailbox/{mailboxID}/filters/new` | New rule form (HTMX partial) |
| POST | `/mailbox/{mailboxID}/filters` | Create rule |
| GET | `/mailbox/{mailboxID}/filters/{ruleID}/edit` | Edit rule form |
| PUT | `/mailbox/{mailboxID}/filters/{ruleID}` | Update rule |
| DELETE | `/mailbox/{mailboxID}/filters/{ruleID}` | Delete rule |
| POST | `/mailbox/{mailboxID}/filters/reorder` | Update priorities via drag-drop |

UI features:
- Rule list with drag-to-reorder (or up/down buttons to keep it simple initially).
- Inline condition builder: field dropdown → operator dropdown → value input.
- Add/remove conditions dynamically via HTMX.
- Toggle active/inactive per rule.

---

## Implementation Order

1. **Migration** — `mailbox_filter_rule` + `mailbox_filter_condition` tables + indexes.
2. **Models** — `FilterRule`, `FilterCondition` structs in `pkg/models/`.
3. **Rule engine** — `internal/filterengine` package with `RuleEngine`, `ParsedMessage`, and condition evaluation logic.
4. **DB layer** — pipeline query + CRUD queries.
5. **Pipeline step** — `step_apply_filter_rules.go` using the rule engine, wired into `pipeline.go`.
6. **Finalize update** — apply filter action to initial email state.
7. **Server routes** — add routes in `server.go`.
8. **Templates** — filter list, rule form, condition row partials.

---

## Out of Scope (for now)

- Applying rules retroactively to existing emails.
- Forwarding/webhook actions.
- Per-rule hit counters or last-matched timestamps.
- Condition on attachment content type or size.
