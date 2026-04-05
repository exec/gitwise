package pagination

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEncodeDecode_Roundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Nanosecond)
	id := uuid.New()

	cursor := EncodeCursor(now, id)
	if cursor == "" {
		t.Fatal("EncodeCursor returned empty string")
	}

	gotTime, gotID, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor error: %v", err)
	}

	if !gotTime.Equal(now) {
		t.Errorf("time = %v, want %v", gotTime, now)
	}
	if gotID != id {
		t.Errorf("id = %v, want %v", gotID, id)
	}
}

func TestEncodeDecode_MultipleValues(t *testing.T) {
	tests := []struct {
		name string
		time time.Time
		id   uuid.UUID
	}{
		{
			name: "zero time with nil UUID",
			time: time.Time{},
			id:   uuid.Nil,
		},
		{
			name: "specific time",
			time: time.Date(2024, 6, 15, 10, 30, 0, 123456789, time.UTC),
			id:   uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		},
		{
			name: "epoch time",
			time: time.Unix(0, 0).UTC(),
			id:   uuid.New(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursor := EncodeCursor(tt.time, tt.id)
			gotTime, gotID, err := DecodeCursor(cursor)
			if err != nil {
				t.Fatalf("DecodeCursor error: %v", err)
			}
			if !gotTime.Equal(tt.time) {
				t.Errorf("time = %v, want %v", gotTime, tt.time)
			}
			if gotID != tt.id {
				t.Errorf("id = %v, want %v", gotID, tt.id)
			}
		})
	}
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	_, _, err := DecodeCursor("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestDecodeCursor_InvalidCursorFormat(t *testing.T) {
	// Valid base64 but invalid time format
	_, _, err := DecodeCursor("bm90LWEtdGltZXN0YW1w") // "not-a-timestamp"
	if err == nil {
		t.Error("expected error for invalid cursor format")
	}
}

func TestDecodeCursor_InvalidUUID(t *testing.T) {
	// Encode a cursor with valid time but replace UUID with garbage
	// "2024-01-01T00:00:00Z|not-a-uuid" in base64
	_, _, err := DecodeCursor("MjAyNC0wMS0wMVQwMDowMDowMFp8bm90LWEtdXVpZA==")
	if err == nil {
		t.Error("expected error for invalid UUID in cursor")
	}
}

func TestCursors_AreUnique(t *testing.T) {
	now := time.Now().UTC()
	c1 := EncodeCursor(now, uuid.New())
	c2 := EncodeCursor(now, uuid.New())
	if c1 == c2 {
		t.Error("cursors with different IDs should be different")
	}
}
