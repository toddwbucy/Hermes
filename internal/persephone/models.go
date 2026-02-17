package persephone

import "time"

// Task status constants matching Persephone's workflow state machine.
const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusInReview   = "in_review"
	StatusBlocked    = "blocked"
	StatusClosed     = "closed"
)

// Task priority constants.
const (
	PriorityLow      = "low"
	PriorityMedium   = "medium"
	PriorityHigh     = "high"
	PriorityCritical = "critical"
)

// Task type constants.
const (
	TypeTask = "task"
	TypeBug  = "bug"
	TypeEpic = "epic"
)

// Edge type constants matching Persephone's edge types.
const (
	EdgeImplements      = "implements"
	EdgeSubmittedReview = "submitted_review"
	EdgeApproved        = "approved"
	EdgeBlockedBy       = "blocked_by"
	EdgeContinues       = "continues"
	EdgeAuthoredHandoff = "authored_handoff"
	EdgeHandoffFor      = "handoff_for"
)

// Task represents a Persephone task node.
type Task struct {
	Key         string    `json:"_key"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority,omitempty"`
	Type        string    `json:"type,omitempty"`
	Labels      []string  `json:"labels,omitempty"`
	ParentKey   string    `json:"parent_key,omitempty"`
	Acceptance  string    `json:"acceptance,omitempty"`
	Minor       bool      `json:"minor,omitempty"`
	BlockReason string    `json:"block_reason,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Session represents a Persephone session node.
type Session struct {
	Key          string     `json:"_key"`
	AgentType    string     `json:"agent_type,omitempty"`
	AgentPID     int        `json:"agent_pid,omitempty"`
	ContextID    string     `json:"context_id,omitempty"`
	Branch       string     `json:"branch,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	LastActivity time.Time  `json:"last_activity,omitempty"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
}

// Edge represents a Persephone graph edge.
type Edge struct {
	Key       string    `json:"_key"`
	From      string    `json:"_from"`
	To        string    `json:"_to"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

// Handoff represents a Persephone handoff node.
type Handoff struct {
	Key        string    `json:"_key"`
	TaskKey    string    `json:"task_key,omitempty"`
	SessionKey string    `json:"session_key,omitempty"`
	Done       []string  `json:"done,omitempty"`
	Remaining  []string  `json:"remaining,omitempty"`
	Decisions  []string  `json:"decisions,omitempty"`
	Uncertain  []string  `json:"uncertain,omitempty"`
	Note       string    `json:"note,omitempty"`
	GitBranch  string    `json:"git_branch,omitempty"`
	GitSHA     string    `json:"git_sha,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
