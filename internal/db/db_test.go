package db

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func setupTest(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, Setup(db))
	return db
}

func TestUser(t *testing.T) {
	db := setupTest(t)

	i, err := AddUser(db, "zdgeier")
	require.NoError(t, err)
	require.Equal(t, i, uint64(1))

	i, err = AddUser(db, "zdgeier2")
	require.NoError(t, err)
	require.Equal(t, i, uint64(2))
}

func TestProject(t *testing.T) {
	db := setupTest(t)

	userId, err := AddUser(db, "zdgeier")
	require.NoError(t, err)

	p1, err := AddProject(db, "testproject", userId)
	require.NoError(t, err)
	require.Equal(t, p1, uint64(1))

	p2, err := AddProject(db, "testproject2", userId)
	require.NoError(t, err)
	require.Equal(t, p2, uint64(2))

	name, ownerUserId, err := GetProject(db, p2)
	require.NoError(t, err)
	require.Equal(t, name, "testproject2")
	require.Equal(t, ownerUserId, userId)
}
