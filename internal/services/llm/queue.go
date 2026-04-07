package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Priority levels for agent tasks. Lower number = higher priority.
const (
	PriorityChat     = 1 // Interactive chat (user is waiting)
	PriorityPush     = 2 // Push-triggered review (near-realtime)
	PrioritySchedule = 3 // Background/scheduled tasks
)

// Redis key prefixes for task queues (one list per priority level).
const (
	queueKeyPrefix = "gitwise:agent:queue:"
)

func queueKey(priority int) string {
	return fmt.Sprintf("%s%d", queueKeyPrefix, priority)
}

// AgentTask is a task submitted to the agent queue.
type AgentTask struct {
	ID           uuid.UUID `json:"id"`
	RepoID       uuid.UUID `json:"repo_id"`
	AgentID      uuid.UUID `json:"agent_id"`
	TriggerEvent string    `json:"trigger_event"`
	TriggerRef   string    `json:"trigger_ref,omitempty"`
	Priority     int       `json:"priority"`
	Provider     string    `json:"provider"`

	// Input for the LLM call
	SystemPrompt string    `json:"system_prompt"`
	Messages     []Message `json:"messages"`
	MaxTokens    int       `json:"max_tokens,omitempty"`
	Temperature  float64   `json:"temperature,omitempty"`
}

// Queue is a Redis-backed task queue with provider-aware concurrency.
type Queue struct {
	rdb     *redis.Client
	db      *pgxpool.Pool
	gateway *Gateway

	numWorkers int
	stopCh     chan struct{}
	wg         sync.WaitGroup

	// ollamaMu enforces sequential execution for local Ollama.
	ollamaMu sync.Mutex
}

// NewQueue creates a new agent task queue.
func NewQueue(rdb *redis.Client, db *pgxpool.Pool, gateway *Gateway, numWorkers int) *Queue {
	if numWorkers <= 0 {
		numWorkers = 4
	}
	return &Queue{
		rdb:        rdb,
		db:         db,
		gateway:    gateway,
		numWorkers: numWorkers,
		stopCh:     make(chan struct{}),
	}
}

// Enqueue adds a task to the queue at the given priority level.
// It also inserts a row into the agent_tasks table with status "queued".
func (q *Queue) Enqueue(ctx context.Context, task AgentTask) error {
	if task.ID == uuid.Nil {
		task.ID = uuid.New()
	}

	// Insert into agent_tasks table
	_, err := q.db.Exec(ctx, `
		INSERT INTO agent_tasks (id, repo_id, agent_id, trigger_event, trigger_ref, status, provider)
		VALUES ($1, $2, $3, $4, $5, 'queued', $6)`,
		task.ID, task.RepoID, task.AgentID, task.TriggerEvent, task.TriggerRef, task.Provider,
	)
	if err != nil {
		return fmt.Errorf("insert agent task: %w", err)
	}

	// Serialize and push to Redis
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal agent task: %w", err)
	}

	key := queueKey(task.Priority)
	if err := q.rdb.LPush(ctx, key, data).Err(); err != nil {
		return fmt.Errorf("enqueue agent task: %w", err)
	}

	slog.Info("agent task enqueued",
		"task_id", task.ID,
		"agent_id", task.AgentID,
		"event", task.TriggerEvent,
		"priority", task.Priority,
	)
	return nil
}

// StartWorkers spawns worker goroutines that dequeue and process tasks.
func (q *Queue) StartWorkers(ctx context.Context) {
	if !q.gateway.IsEnabled() {
		slog.Info("llm gateway disabled, agent queue workers not started")
		return
	}

	slog.Info("starting agent queue workers", "count", q.numWorkers)
	for i := 0; i < q.numWorkers; i++ {
		q.wg.Add(1)
		go q.worker(ctx, i)
	}
}

// StopWorkers signals all workers to stop and waits for them to finish.
func (q *Queue) StopWorkers() {
	close(q.stopCh)
	q.wg.Wait()
	slog.Info("agent queue workers stopped")
}

// worker is the main loop for a single queue worker.
func (q *Queue) worker(ctx context.Context, id int) {
	defer q.wg.Done()
	slog.Debug("agent queue worker started", "worker_id", id)

	for {
		select {
		case <-q.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		task, err := q.dequeue(ctx)
		if err != nil {
			// BRPOP timeout (nil result) — just retry
			continue
		}

		q.processTask(ctx, task)
	}
}

// dequeue pops the highest-priority task from the queue.
// It checks priority 1 first, then 2, then 3. Uses BRPOP with a 1-second
// timeout so workers can check the stop channel regularly.
func (q *Queue) dequeue(ctx context.Context) (*AgentTask, error) {
	// Try each priority level in order (highest first).
	// Use RPOP (non-blocking) for priority 1 and 2, then BRPOP on priority 3
	// with a short timeout so we don't starve high-priority tasks.
	for _, prio := range []int{PriorityChat, PriorityPush} {
		result, err := q.rdb.RPop(ctx, queueKey(prio)).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, err
		}
		var task AgentTask
		if err := json.Unmarshal([]byte(result), &task); err != nil {
			slog.Warn("agent queue: unmarshal error", "error", err)
			continue
		}
		return &task, nil
	}

	// Block on the lowest priority queue with a short timeout.
	results, err := q.rdb.BRPop(ctx, 1*time.Second, queueKey(PrioritySchedule)).Result()
	if err != nil {
		return nil, err
	}
	if len(results) < 2 {
		return nil, fmt.Errorf("unexpected BRPOP result length")
	}

	var task AgentTask
	if err := json.Unmarshal([]byte(results[1]), &task); err != nil {
		return nil, fmt.Errorf("unmarshal agent task: %w", err)
	}
	return &task, nil
}

// processTask executes a single agent task.
func (q *Queue) processTask(ctx context.Context, task *AgentTask) {
	slog.Info("processing agent task",
		"task_id", task.ID,
		"agent_id", task.AgentID,
		"event", task.TriggerEvent,
	)

	// Mark as running
	_, err := q.db.Exec(ctx, `
		UPDATE agent_tasks SET status = 'running' WHERE id = $1`, task.ID)
	if err != nil {
		slog.Error("failed to update task status", "task_id", task.ID, "error", err)
	}

	// Enforce sequential execution for local Ollama
	provider := q.gateway.Provider()
	if provider != nil && !provider.SupportsParallel() {
		q.ollamaMu.Lock()
		defer q.ollamaMu.Unlock()
	}

	start := time.Now()

	req := GenerateRequest{
		SystemPrompt: task.SystemPrompt,
		Messages:     task.Messages,
		MaxTokens:    task.MaxTokens,
		Temperature:  task.Temperature,
	}

	resp, err := q.gateway.Generate(ctx, req)
	duration := time.Since(start)

	if err != nil {
		slog.Error("agent task failed",
			"task_id", task.ID,
			"error", err,
			"duration_ms", duration.Milliseconds(),
		)
		_, dbErr := q.db.Exec(ctx, `
			UPDATE agent_tasks
			SET status = 'failed', error = $2, duration_ms = $3, completed_at = now()
			WHERE id = $1`,
			task.ID, err.Error(), duration.Milliseconds())
		if dbErr != nil {
			slog.Error("failed to update failed task", "task_id", task.ID, "error", dbErr)
		}
		return
	}

	// Store result
	result, _ := json.Marshal(map[string]string{"content": resp.Content})
	_, err = q.db.Exec(ctx, `
		UPDATE agent_tasks
		SET status = 'completed',
		    input_tokens = $2,
		    output_tokens = $3,
		    duration_ms = $4,
		    result = $5,
		    completed_at = now()
		WHERE id = $1`,
		task.ID, resp.InputTokens, resp.OutputTokens, duration.Milliseconds(), result)
	if err != nil {
		slog.Error("failed to update completed task", "task_id", task.ID, "error", err)
	}

	slog.Info("agent task completed",
		"task_id", task.ID,
		"input_tokens", resp.InputTokens,
		"output_tokens", resp.OutputTokens,
		"duration_ms", duration.Milliseconds(),
	)
}
