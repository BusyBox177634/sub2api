package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestEnsureCodexFingerprintSeed_ReturnsAtomicallyStoredValue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	candidate := "11111111-1111-4111-8111-111111111111"
	existing := "22222222-2222-4222-8222-222222222222"
	mock.ExpectQuery(`(?s)UPDATE accounts.*codex_fingerprint_seed.*RETURNING`).
		WithArgs(candidate, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"codex_fingerprint_seed"}).AddRow(existing))

	repo := newAccountRepositoryWithSQL(client, db, nil)
	seed, err := repo.EnsureCodexFingerprintSeed(context.Background(), 42, candidate)
	require.NoError(t, err)
	require.Equal(t, existing, seed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureCodexFingerprintSeed_RejectsInvalidCandidate(t *testing.T) {
	repo := newAccountRepositoryWithSQL(nil, nil, nil)
	_, err := repo.EnsureCodexFingerprintSeed(context.Background(), 42, "not-a-uuid")
	require.Error(t, err)
}

func TestBulkUpdate_IgnoresCodexFingerprintSeed(t *testing.T) {
	repo := newAccountRepositoryWithSQL(nil, nil, nil)
	updates := service.AccountBulkUpdate{
		Extra: map[string]any{service.CodexFingerprintSeedExtraKey: "44444444-4444-4444-8444-444444444444"},
	}
	updated, err := repo.BulkUpdate(context.Background(), []int64{42}, updates)
	require.NoError(t, err)
	require.Zero(t, updated)
}

func TestLockAndMergeAccountProbeExtra_PreservesStoredCodexFingerprintSeed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	storedSeed := "33333333-3333-4333-8333-333333333333"
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
		WithArgs(int64(43), service.PlatformOpenAI, service.AccountTypeAPIKey, `{"api_key":"sk-test"}`, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"identity_unchanged", "ollama_group_unchanged", "ollama_proxy_unchanged", "enabled", "rate_sync_enabled",
			"snapshot", "ollama_session", "ollama_auto", "ollama_snapshot", "codex_seed",
		}).AddRow(true, false, true, nil, nil, nil, nil, nil, nil, storedSeed))

	account := &service.Account{
		ID:          43,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			service.CodexFingerprintSeedExtraKey: "44444444-4444-4444-8444-444444444444",
		},
	}
	extra, err := lockAndMergeAccountProbeExtra(context.Background(), client, account, nil, nil)
	require.NoError(t, err)
	require.Equal(t, storedSeed, extra[service.CodexFingerprintSeedExtraKey])
	require.NoError(t, mock.ExpectationsWereMet())
}
