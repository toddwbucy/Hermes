package persephone

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/toddwbucy/hermes/internal/arango"
)

// Store provides typed access to Persephone collections in ArangoDB.
type Store struct {
	client *arango.Client
}

// NewStore creates a new Persephone store wrapping an ArangoDB client.
func NewStore(client *arango.Client) *Store {
	return &Store{client: client}
}

// Ping tests connectivity to the database.
func (s *Store) Ping() error {
	return s.client.Ping()
}

// Database returns the configured database name.
func (s *Store) Database() string {
	return s.client.Database()
}

// ListTasks returns tasks, optionally filtered by status.
// If no statuses provided, returns all tasks.
func (s *Store) ListTasks(statuses ...string) ([]Task, error) {
	var aql string
	var bindVars map[string]any

	if len(statuses) > 0 {
		aql = `FOR doc IN persephone_tasks
			FILTER doc.status IN @statuses
			SORT doc.updated_at DESC
			RETURN doc`
		bindVars = map[string]any{"statuses": statuses}
	} else {
		aql = `FOR doc IN persephone_tasks
			SORT doc.updated_at DESC
			RETURN doc`
	}

	return queryTyped[Task](s.client, aql, bindVars)
}

// GetTask returns a single task by key.
func (s *Store) GetTask(key string) (*Task, error) {
	aql := `FOR doc IN persephone_tasks
		FILTER doc._key == @key
		LIMIT 1
		RETURN doc`
	results, err := queryTyped[Task](s.client, aql, map[string]any{"key": key})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("task not found: %s", key)
	}
	return &results[0], nil
}

// TaskEdges returns all edges connected to a task.
func (s *Store) TaskEdges(taskKey string) ([]Edge, error) {
	aql := `FOR e IN persephone_edges
		FILTER e._from == @id OR e._to == @id
		SORT e.created_at DESC
		RETURN e`
	id := "persephone_tasks/" + taskKey
	return queryTyped[Edge](s.client, aql, map[string]any{"id": id})
}

// LatestHandoff returns the most recent handoff for a task.
func (s *Store) LatestHandoff(taskKey string) (*Handoff, error) {
	aql := `FOR doc IN persephone_handoffs
		FILTER doc.task_key == @taskKey
		SORT doc.created_at DESC
		LIMIT 1
		RETURN doc`
	results, err := queryTyped[Handoff](s.client, aql, map[string]any{"taskKey": taskKey})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

// TasksByStatus returns tasks grouped by status.
func (s *Store) TasksByStatus() (map[string][]Task, error) {
	tasks, err := s.ListTasks()
	if err != nil {
		return nil, err
	}

	grouped := make(map[string][]Task)
	for _, t := range tasks {
		grouped[t.Status] = append(grouped[t.Status], t)
	}
	return grouped, nil
}

// TaskCounts returns count of tasks per status.
func (s *Store) TaskCounts() (map[string]int, error) {
	aql := `FOR doc IN persephone_tasks
		COLLECT status = doc.status WITH COUNT INTO cnt
		RETURN {status, cnt}`

	type statusCount struct {
		Status string `json:"status"`
		Count  int    `json:"cnt"`
	}

	results, err := queryTyped[statusCount](s.client, aql, nil)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.Status] = r.Count
	}
	return counts, nil
}

// TaskSessions returns sessions that have an "implements" edge to the given task.
func (s *Store) TaskSessions(taskKey string) ([]Session, error) {
	aql := `FOR e IN persephone_edges
		FILTER e._to == @id AND e.type == "implements"
		FOR s IN persephone_sessions
			FILTER s._id == e._from
			SORT s.started_at DESC
			RETURN s`
	id := "persephone_tasks/" + taskKey
	return queryTyped[Session](s.client, aql, map[string]any{"id": id})
}

// ValidTransitions defines the legal workflow state transitions,
// mirroring Persephone's workflow.py state machine exactly.
var ValidTransitions = map[string][]string{
	StatusOpen:       {StatusInProgress},
	StatusInProgress: {StatusInReview, StatusBlocked, StatusOpen},
	StatusBlocked:    {StatusInProgress, StatusOpen},
	StatusInReview:   {StatusClosed, StatusInProgress},
	StatusClosed:     {StatusOpen},
}

// TransitionTask changes a task's status after validating the transition is legal.
// If newStatus is "blocked", blockReason should be non-empty.
func (s *Store) TransitionTask(taskKey, newStatus, blockReason string) error {
	// Fetch current task to validate transition
	task, err := s.GetTask(taskKey)
	if err != nil {
		return fmt.Errorf("get task for transition: %w", err)
	}

	// Validate transition
	allowed, ok := ValidTransitions[task.Status]
	if !ok {
		return fmt.Errorf("no transitions defined from status %q", task.Status)
	}
	valid := false
	for _, s := range allowed {
		if s == newStatus {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid transition: %s → %s", task.Status, newStatus)
	}

	// Build update fields
	fields := map[string]any{
		"status":     newStatus,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if newStatus == StatusBlocked {
		fields["block_reason"] = blockReason
	} else if task.Status == StatusBlocked {
		// Clear block reason when leaving blocked state
		fields["block_reason"] = ""
	}

	return s.client.UpdateDocument("persephone_tasks", taskKey, fields)
}

// UpdateTaskField updates arbitrary fields on a task document.
func (s *Store) UpdateTaskField(taskKey string, fields map[string]any) error {
	return s.client.UpdateDocument("persephone_tasks", taskKey, fields)
}

// queryTyped executes an AQL query and unmarshals results into typed slice.
func queryTyped[T any](client *arango.Client, aql string, bindVars map[string]any) ([]T, error) {
	raw, err := client.Query(aql, bindVars)
	if err != nil {
		return nil, err
	}

	results := make([]T, 0, len(raw))
	for _, r := range raw {
		var item T
		if err := json.Unmarshal(r, &item); err != nil {
			return results, fmt.Errorf("unmarshal result: %w", err)
		}
		results = append(results, item)
	}
	return results, nil
}
