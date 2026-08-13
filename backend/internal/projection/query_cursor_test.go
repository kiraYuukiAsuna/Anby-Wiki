package projection

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/page"
)

func TestSourceUsageCursorRoundTrip(t *testing.T) {
	pageID := uuid.New()
	cursor := encodeSourceUsageCursor(pageID)
	decoded, err := sourceUsageCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if decoded == nil || *decoded != pageID {
		t.Fatalf("decoded=%v, want %s", decoded, pageID)
	}
}

func TestSourceUsageCursorRejectsUsageLocationCursor(t *testing.T) {
	cursor := encodeUsageCursor(uuid.New(), uuid.New(), "1")
	if _, err := sourceUsageCursor(cursor); !errors.Is(err, page.ErrInvalidCursor) {
		t.Fatalf("error=%v, want ErrInvalidCursor", err)
	}
}
