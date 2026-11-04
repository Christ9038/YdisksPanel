package store

import (
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID           int64   `json:"id"`
	Username     string  `json:"username"`
	DisplayName  string  `json:"displayName"`
	Role         string  `json:"role"`
	Disabled     bool    `json:"disabled"`
	TOTPEnabled  bool    `json:"totpEnabled"`
	CreatedAt    int64   `json:"createdAt"`
	Teams        []int64 `json:"teams"`
	PasswordHash string  `json:"-"`
	TOTPSecret   string  `json:"-"`
	TOTPLastStep int64   `json:"-"`
}

const userCols = `id,username,display_name,password_hash,role,disabled,totp_secret,totp_enabled,totp_last_step,created_at`

func scanUser(r interface{ Scan(...any) error }) (*User, error) {
	var u User
	var dis, tot int
	if err := r.Scan(&u.ID, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Role, &dis, &u.TOTPSecret, &tot, &u.TOTPLastStep, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Disabled, u.TOTPEnabled = dis == 1, tot == 1
	u.Teams = []int64{}
	return &u, nil
}

func (s *Store) loadTeams(u *User) {
	rows, err := s.db.Query(`SELECT team_id FROM user_teams WHERE user_id=?`, u.ID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			u.Teams = append(u.Teams, id)
		}
	}
}

func (s *Store) CountUsers() (n int, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return
}

func (s *Store) CreateUser(username, display, hash, role string) (*User, error) {
	res, err := s.db.Exec(`INSERT INTO users(username,display_name,password_hash,role,created_at) VALUES(?,?,?,?,?)`,
		username, display, hash, role, now())
	if isUnique(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetUser(id)
}

func (s *Store) GetUser(id int64) (*User, error) {
	u, err := scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id=?`, id))
	if err == nil {
		s.loadTeams(u)
	}
	return u, err
}

func (s *Store) GetUserByName(name string) (*User, error) {
	u, err := scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE username=?`, name))
	if err == nil {
		s.loadTeams(u)
	}
	return u, err
}

func (s *Store) ListUsers() ([]*User, error) {
	rows, err := s.db.Query(`SELECT ` + userCols + ` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, u)
	}
	rows.Close()
	for _, u := range out {
		s.loadTeams(u)
	}
	return out, nil
}

func (s *Store) CountActiveAdmins() (n int, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin' AND disabled=0`).Scan(&n)
	return
}

func (s *Store) UpdateUser(id int64, display, role string, disabled bool) error {
	_, err := s.db.Exec(`UPDATE users SET display_name=?,role=?,disabled=? WHERE id=?`, display, role, b2i(disabled), id)
	return err
}

func (s *Store) SetUserTeams(id int64, teams []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM user_teams WHERE user_id=?`, id); err != nil {
		return err
	}
	for _, t := range teams {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO user_teams VALUES(?,?)`, id, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetPassword(id int64, hash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, id)
	return err
}

func (s *Store) DeleteUser(id int64) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE id=?`, id)
	return err
}

// ---- TOTP ----

func (s *Store) SetTOTPSecret(id int64, secret string) error {
	_, err := s.db.Exec(`UPDATE users SET totp_secret=? WHERE id=? AND totp_enabled=0`, secret, id)
	return err
}

func (s *Store) EnableTOTP(id int64, codeHashes []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET totp_enabled=1 WHERE id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM recovery_codes WHERE user_id=?`, id); err != nil {
		return err
	}
	for _, h := range codeHashes {
		if _, err := tx.Exec(`INSERT INTO recovery_codes(user_id,hash) VALUES(?,?)`, id, h); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DisableTOTP(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET totp_enabled=0,totp_secret='',totp_last_step=0 WHERE id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM recovery_codes WHERE user_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ClaimTOTPStep atomically records a used time step so a code can't be replayed.
func (s *Store) ClaimTOTPStep(id, step int64) (bool, error) {
	res, err := s.db.Exec(`UPDATE users SET totp_last_step=? WHERE id=? AND totp_last_step<?`, step, id, step)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ConsumeRecoveryCode marks the code with this hash used; false if none matched.
func (s *Store) ConsumeRecoveryCode(userID int64, hash string) (bool, error) {
	res, err := s.db.Exec(`UPDATE recovery_codes SET used=1 WHERE user_id=? AND hash=? AND used=0`, userID, hash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ---- sessions & login challenges ----

type Session struct {
	ID        string `json:"id"`
	UserID    int64  `json:"-"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
	LastSeen  int64  `json:"lastSeen"`
	TTL       int64  `json:"-"`
	IP        string `json:"ip"`
	UserAgent string `json:"userAgent"`
	Current   bool   `json:"current"`
}

func (s *Store) CreateSession(id string, userID int64, ttl time.Duration, ip, ua string) error {
	t := now()
	_, err := s.db.Exec(`INSERT INTO sessions(id,user_id,created_at,expires_at,last_seen,ttl,ip,user_agent) VALUES(?,?,?,?,?,?,?,?)`,
		id, userID, t, t+int64(ttl.Seconds()), t, int64(ttl.Seconds()), ip, ua)
	return err
}

func (s *Store) GetSession(id string) (*Session, error) {
	var x Session
	err := s.db.QueryRow(`SELECT id,user_id,created_at,expires_at,last_seen,ttl,ip,user_agent FROM sessions WHERE id=?`, id).
		Scan(&x.ID, &x.UserID, &x.CreatedAt, &x.ExpiresAt, &x.LastSeen, &x.TTL, &x.IP, &x.UserAgent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &x, err
}

func (s *Store) TouchSession(x *Session) error {
	t := now()
	_, err := s.db.Exec(`UPDATE sessions SET last_seen=?,expires_at=? WHERE id=?`, t, t+x.TTL, x.ID)
	return err
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id=?`, id)
	return err
}

// DeleteUserSessions revokes a user's sessions, optionally keeping one.
func (s *Store) DeleteUserSessions(userID int64, keep string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id=? AND id<>?`, userID, keep)
	return err
}

func (s *Store) DeleteUserSession(userID int64, id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id=? AND id=?`, userID, id)
	return err
}

func (s *Store) ListSessions(userID int64) ([]Session, error) {
	rows, err := s.db.Query(`SELECT id,created_at,expires_at,last_seen,ip,user_agent FROM sessions WHERE user_id=? AND expires_at>? ORDER BY last_seen DESC`, userID, now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var x Session
		if err := rows.Scan(&x.ID, &x.CreatedAt, &x.ExpiresAt, &x.LastSeen, &x.IP, &x.UserAgent); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) PurgeExpired() {
	t := now()
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at<?`, t)
	_, _ = s.db.Exec(`DELETE FROM challenges WHERE expires_at<?`, t)
}

type Challenge struct {
	ID       string
	UserID   int64
	Remember bool
	Attempts int
}

func (s *Store) CreateChallenge(id string, userID int64, remember bool, ttl time.Duration) error {
	_, err := s.db.Exec(`INSERT INTO challenges(id,user_id,remember,expires_at) VALUES(?,?,?,?)`,
		id, userID, b2i(remember), now()+int64(ttl.Seconds()))
	return err
}

func (s *Store) GetChallenge(id string) (*Challenge, error) {
	var c Challenge
	var rem int
	var exp int64
	err := s.db.QueryRow(`SELECT id,user_id,remember,attempts,expires_at FROM challenges WHERE id=?`, id).
		Scan(&c.ID, &c.UserID, &rem, &c.Attempts, &exp)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && exp < now()) {
		return nil, ErrNotFound
	}
	c.Remember = rem == 1
	return &c, err
}

func (s *Store) BumpChallenge(id string) error {
	_, err := s.db.Exec(`UPDATE challenges SET attempts=attempts+1 WHERE id=?`, id)
	return err
}

func (s *Store) DeleteChallenge(id string) error {
	_, err := s.db.Exec(`DELETE FROM challenges WHERE id=?`, id)
	return err
}
