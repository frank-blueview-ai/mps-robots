package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type UserProfile struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Role        string    `json:"role"`
	Email       string    `json:"email,omitempty"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ClassProfile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Term      string    `json:"term,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s *ProjectStore) ListUsers() ([]UserProfile, error) {
	rows, err := s.db.Query(`
SELECT id, display_name, role, email, active, created_at, updated_at
FROM users
ORDER BY active DESC, lower(display_name)
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []UserProfile{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

func (s *ProjectStore) CreateUser(user UserProfile) (UserProfile, error) {
	now := time.Now().UTC()
	user.ID = newEntityID("u")
	user.DisplayName = strings.TrimSpace(user.DisplayName)
	user.Role = normalizeUserRole(user.Role)
	user.Email = strings.TrimSpace(user.Email)
	user.Active = true
	user.CreatedAt = now
	user.UpdatedAt = now

	if user.DisplayName == "" {
		return UserProfile{}, errors.New("display name is required")
	}
	if !validUserRole(user.Role) {
		return UserProfile{}, fmt.Errorf("invalid role %q", user.Role)
	}

	_, err := s.db.Exec(`
INSERT INTO users (id, display_name, role, email, active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, user.ID, user.DisplayName, user.Role, user.Email, boolToInt(user.Active), formatDBTime(user.CreatedAt), formatDBTime(user.UpdatedAt))
	if err != nil {
		return UserProfile{}, err
	}

	if err := s.audit("user.created", "user", user.ID, map[string]string{"displayName": user.DisplayName, "role": user.Role}); err != nil {
		return UserProfile{}, err
	}

	return user, nil
}

func (s *ProjectStore) GetUser(id string) (UserProfile, error) {
	row := s.db.QueryRow(`
SELECT id, display_name, role, email, active, created_at, updated_at
FROM users
WHERE id = ?
`, id)
	return scanUser(row)
}

func (s *ProjectStore) UpdateUser(id string, user UserProfile) (UserProfile, error) {
	existing, err := s.GetUser(id)
	if err != nil {
		return UserProfile{}, err
	}

	user.ID = id
	user.DisplayName = firstNonEmpty(strings.TrimSpace(user.DisplayName), existing.DisplayName)
	user.Role = normalizeUserRole(firstNonEmpty(user.Role, existing.Role))
	user.Email = strings.TrimSpace(user.Email)
	user.Active = user.Active || !isExplicitlyInactive(user)
	user.CreatedAt = existing.CreatedAt
	user.UpdatedAt = time.Now().UTC()

	if !validUserRole(user.Role) {
		return UserProfile{}, fmt.Errorf("invalid role %q", user.Role)
	}

	_, err = s.db.Exec(`
UPDATE users
SET display_name = ?, role = ?, email = ?, active = ?, updated_at = ?
WHERE id = ?
`, user.DisplayName, user.Role, user.Email, boolToInt(user.Active), formatDBTime(user.UpdatedAt), id)
	if err != nil {
		return UserProfile{}, err
	}

	if err := s.audit("user.updated", "user", user.ID, map[string]string{"displayName": user.DisplayName, "role": user.Role}); err != nil {
		return UserProfile{}, err
	}

	return user, nil
}

func (s *ProjectStore) DeleteUser(id string) error {
	if id == "station-admin" {
		return errors.New("station admin cannot be deleted")
	}

	result, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	return s.audit("user.deleted", "user", id, map[string]string{})
}

func (s *ProjectStore) ListClasses() ([]ClassProfile, error) {
	rows, err := s.db.Query(`
SELECT id, name, term, created_at, updated_at
FROM classes
ORDER BY lower(name)
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	classes := []ClassProfile{}
	for rows.Next() {
		class, err := scanClass(rows)
		if err != nil {
			return nil, err
		}
		classes = append(classes, class)
	}

	return classes, rows.Err()
}

func (s *ProjectStore) CreateClass(class ClassProfile) (ClassProfile, error) {
	now := time.Now().UTC()
	class.ID = newEntityID("c")
	class.Name = strings.TrimSpace(class.Name)
	class.Term = strings.TrimSpace(class.Term)
	class.CreatedAt = now
	class.UpdatedAt = now

	if class.Name == "" {
		return ClassProfile{}, errors.New("class name is required")
	}

	_, err := s.db.Exec(`
INSERT INTO classes (id, name, term, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
`, class.ID, class.Name, class.Term, formatDBTime(class.CreatedAt), formatDBTime(class.UpdatedAt))
	if err != nil {
		return ClassProfile{}, err
	}

	if err := s.audit("class.created", "class", class.ID, map[string]string{"name": class.Name}); err != nil {
		return ClassProfile{}, err
	}

	return class, nil
}

func (s *ProjectStore) GetClass(id string) (ClassProfile, error) {
	row := s.db.QueryRow(`
SELECT id, name, term, created_at, updated_at
FROM classes
WHERE id = ?
`, id)
	return scanClass(row)
}

func (s *ProjectStore) UpdateClass(id string, class ClassProfile) (ClassProfile, error) {
	existing, err := s.GetClass(id)
	if err != nil {
		return ClassProfile{}, err
	}

	class.ID = id
	class.Name = firstNonEmpty(strings.TrimSpace(class.Name), existing.Name)
	class.Term = strings.TrimSpace(class.Term)
	class.CreatedAt = existing.CreatedAt
	class.UpdatedAt = time.Now().UTC()

	_, err = s.db.Exec(`
UPDATE classes
SET name = ?, term = ?, updated_at = ?
WHERE id = ?
`, class.Name, class.Term, formatDBTime(class.UpdatedAt), id)
	if err != nil {
		return ClassProfile{}, err
	}

	if err := s.audit("class.updated", "class", class.ID, map[string]string{"name": class.Name}); err != nil {
		return ClassProfile{}, err
	}

	return class, nil
}

func (s *ProjectStore) DeleteClass(id string) error {
	result, err := s.db.Exec(`DELETE FROM classes WHERE id = ?`, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	return s.audit("class.deleted", "class", id, map[string]string{})
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanUser(row scanner) (UserProfile, error) {
	var user UserProfile
	var active int
	var createdAt string
	var updatedAt string

	err := row.Scan(&user.ID, &user.DisplayName, &user.Role, &user.Email, &active, &createdAt, &updatedAt)
	if err != nil {
		return UserProfile{}, err
	}

	user.Active = active != 0
	user.CreatedAt = parseDBTime(createdAt)
	user.UpdatedAt = parseDBTime(updatedAt)
	return user, nil
}

func scanClass(row scanner) (ClassProfile, error) {
	var class ClassProfile
	var createdAt string
	var updatedAt string

	err := row.Scan(&class.ID, &class.Name, &class.Term, &createdAt, &updatedAt)
	if err != nil {
		return ClassProfile{}, err
	}

	class.CreatedAt = parseDBTime(createdAt)
	class.UpdatedAt = parseDBTime(updatedAt)
	return class, nil
}

func normalizeUserRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return "student"
	}
	return role
}

func validUserRole(role string) bool {
	switch role {
	case "admin", "teacher", "student", "operator":
		return true
	default:
		return false
	}
}

func isExplicitlyInactive(user UserProfile) bool {
	return !user.Active
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func newEntityID(prefix string) string {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return prefix + "_" + time.Now().UTC().Format("20060102_150405")
	}
	return prefix + "_" + time.Now().UTC().Format("20060102_150405") + "_" + hex.EncodeToString(random)
}
