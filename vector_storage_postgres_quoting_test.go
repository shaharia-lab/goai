package goai

import (
	"strings"
	"testing"
)

// TestQualifiedTableNameQuoting verifies that schema/collection identifiers are
// escaped via pq.QuoteIdentifier so a hostile collection name cannot break out
// of the identifier position and inject SQL (the defense behind the G201
// suppressions in this file).
func TestQualifiedTableNameQuoting(t *testing.T) {
	p := &PostgresProvider{schema: "public"}

	got := p.qualifiedTableName("docs")
	if want := `"public"."docs"`; got != want {
		t.Errorf("qualifiedTableName(%q) = %q, want %q", "docs", got, want)
	}

	// A name attempting to inject a statement must be neutralised: the whole
	// thing stays a single quoted identifier and any embedded double-quote is
	// doubled (pq escaping), so no SQL metacharacter escapes the identifier.
	hostile := `docs"; DROP TABLE users; --`
	quoted := p.qualifiedTableName(hostile)
	if want := `"public"."docs""; DROP TABLE users; --"`; quoted != want {
		t.Fatalf("qualifiedTableName(hostile) = %q, want %q", quoted, want)
	}
	// The hostile input's embedded quote must be doubled (escaped), and the
	// dangerous unescaped sequence `docs";` (which would terminate the
	// identifier early) must NOT appear.
	if strings.Contains(quoted, `docs";`) {
		t.Errorf("embedded quote not escaped — identifier can be broken out of: %q", quoted)
	}
}

func TestQuotedSchema(t *testing.T) {
	p := &PostgresProvider{schema: "my_schema"}
	if got, want := p.quotedSchema(), `"my_schema"`; got != want {
		t.Errorf("quotedSchema() = %q, want %q", got, want)
	}
}
