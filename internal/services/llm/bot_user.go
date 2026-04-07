package llm

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	botUsername = "gitwise-bot"
	botEmail   = "bot@gitwise.local"
)

// EnsureBotUser creates the gitwise-bot user if it does not already exist.
// Returns the bot user's ID. Called during server startup.
func EnsureBotUser(ctx context.Context, db *pgxpool.Pool) (uuid.UUID, error) {
	var id uuid.UUID
	err := db.QueryRow(ctx, `
		SELECT id FROM users WHERE username = $1`, botUsername).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("lookup bot user: %w", err)
	}

	// Create the bot user (no password — cannot log in via normal auth)
	err = db.QueryRow(ctx, `
		INSERT INTO users (username, email, is_bot)
		VALUES ($1, $2, true)
		RETURNING id`,
		botUsername, botEmail).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create bot user: %w", err)
	}

	return id, nil
}
