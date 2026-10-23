package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type noRowsDB struct{}

func (noRowsDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected Exec")
}

func (noRowsDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected Query")
}

func (noRowsDB) QueryRow(context.Context, string, ...any) pgx.Row { return noRowsRow{} }

type noRowsRow struct{}

func (noRowsRow) Scan(...any) error { return pgx.ErrNoRows }

// An unknown email must spend the same Argon2id work as a known one, otherwise
// response time reveals which emails have local credentials.
func TestVerifyLocalPasswordUnknownEmailRunsDummyHashVerify(t *testing.T) {
	var verified int
	previous := comparePasswordAndHash
	comparePasswordAndHash = func(password, hash string) (bool, error) {
		verified++
		return previous(password, hash)
	}
	t.Cleanup(func() { comparePasswordAndHash = previous })

	repo := &Repository{db: noRowsDB{}}
	if _, _, err := repo.VerifyLocalPassword(t.Context(), "nobody@example.com", "any password"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown email error = %v, want pgx.ErrNoRows", err)
	}
	if verified != 1 {
		t.Fatalf("unknown email ran %d Argon2id verifications, want 1", verified)
	}
}

func TestDummyVerifierUsesRealVerifierParameters(t *testing.T) {
	real, err := secretVerifier("a real password")
	if err != nil {
		t.Fatal(err)
	}
	// Parameter prefix ($argon2id$v=19$m=...,t=...,p=...$) must match so the
	// dummy verify costs the same as a verify against a stored hash.
	if got, want := splitHashParams(string(dummyVerifier())), splitHashParams(string(real)); got != want {
		t.Fatalf("dummy verifier params = %q, want %q", got, want)
	}
}

func splitHashParams(hash string) string {
	count := 0
	for i := range hash {
		if hash[i] == '$' {
			count++
			if count == 4 {
				return hash[:i]
			}
		}
	}
	return hash
}
