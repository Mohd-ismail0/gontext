package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsMissingFTSColumn(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "42703", Message: `column "search_tsv" does not exist`}
	if !isMissingFTSColumn(pgErr) {
		t.Fatal("expected pg undefined_column on search_tsv")
	}
	if isMissingFTSColumn(errors.New("connection refused")) {
		t.Fatal("must not treat generic errors as missing FTS")
	}
	if isMissingFTSColumn(fmt.Errorf("syntax error")) {
		t.Fatal("must not treat syntax errors as missing FTS")
	}
}
