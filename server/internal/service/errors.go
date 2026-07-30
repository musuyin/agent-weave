package service

import (
	"errors"
	"strings"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// ErrSystemReadOnly is returned when a caller tries to modify or delete a
// system-seeded (is_system) row.
var ErrSystemReadOnly = errors.New("system rows are read-only")

// isDuplicateKeyError returns true if err is a MySQL duplicate-key (1062) or
// SQLite unique-constraint error. Used to handle concurrent get-or-create races.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "1062") || // MySQL: Duplicate entry
		strings.Contains(s, "UNIQUE constraint failed") // SQLite
}
