package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

const (
	AccessCookie  = "ekko_access"
	RefreshCookie = "ekko_refresh"
	AccessTTL     = 15 * time.Minute
	RefreshTTL    = 30 * 24 * time.Hour
)

var ErrInvalidCredentials = errors.New("invalid username or password")
var ErrInvalidToken = errors.New("invalid or expired session")
var ErrRefreshReuse = errors.New("refresh token reuse detected")

type Service struct {
	db     *sql.DB
	key    []byte
	now    func() time.Time
	secure bool
}

type Admin struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type Tokens struct {
	Access        string
	Refresh       string
	AccessExpires time.Time
	RefreshExpiry time.Time
	Admin         Admin
}

type claims struct {
	Type      string `json:"typ"`
	AdminID   int64  `json:"aid"`
	Username  string `json:"usr"`
	SessionID string `json:"sid"`
	FamilyID  string `json:"fid,omitempty"`
	jwt.RegisteredClaims
}

func New(db *sql.DB, key []byte, secure bool) (*Service, error) {
	if len(key) < 32 {
		return nil, errors.New("JWT signing key must contain at least 32 bytes")
	}
	return &Service{db: db, key: append([]byte(nil), key...), now: time.Now, secure: secure}, nil
}

func NewFromFile(db *sql.DB, path string, secure bool) (*Service, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read JWT signing key: %w", err)
	}
	return New(db, b, secure)
}

func NormalizeUsername(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func ValidateCredentials(username, password string) error {
	u := NormalizeUsername(username)
	if len(u) < 3 || len(u) > 64 {
		return errors.New("username must be 3..64 characters")
	}
	for _, r := range u {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return errors.New("username contains unsupported characters")
		}
	}
	if len(password) < 12 || len(password) > 1024 {
		return errors.New("password must be 12..1024 characters")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	const memory, iterations, parallelism, keyLen = uint32(64 * 1024), uint32(3), uint8(2), uint32(32)
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	salt, e1 := base64.RawStdEncoding.DecodeString(parts[4])
	expected, e2 := base64.RawStdEncoding.DecodeString(parts[5])
	if e1 != nil || e2 != nil || len(expected) == 0 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func (s *Service) AdminCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins WHERE active=1`).Scan(&n)
	return n, err
}

func (s *Service) CreateInitialAdmin(ctx context.Context, username, password string) error {
	if err := ValidateCredentials(username, password); err != nil {
		return err
	}
	h, err := HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return errors.New("administrator already exists")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO admins(username,password_hash) VALUES(?,?)`, NormalizeUsername(username), h)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ResetPassword(ctx context.Context, username, password string) error {
	if NormalizeUsername(username) == "" {
		if err := s.db.QueryRowContext(ctx, `SELECT username FROM admins WHERE active=1 LIMIT 1`).Scan(&username); err != nil {
			return err
		}
	}
	if err := ValidateCredentials(username, password); err != nil {
		return err
	}
	h, err := HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, `UPDATE admins SET password_hash=?,updated_at=CURRENT_TIMESTAMP WHERE username=? COLLATE NOCASE AND active=1`, h, NormalizeUsername(username))
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,CURRENT_TIMESTAMP) WHERE admin_id=(SELECT id FROM admins WHERE username=? COLLATE NOCASE)`, NormalizeUsername(username)); err != nil {
		return err
	}
	return tx.Commit()
}

func randomID() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func fingerprint(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }

func (s *Service) signed(c claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.key)
}
func (s *Service) parse(raw, typ string) (claims, error) {
	t, err := jwt.ParseWithClaims(raw, &claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return s.key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("ekkoplayer"))
	if err != nil {
		return claims{}, ErrInvalidToken
	}
	x, ok := t.Claims.(*claims)
	if !ok || !t.Valid || x.Type != typ {
		return claims{}, ErrInvalidToken
	}
	return *x, nil
}

func (s *Service) issue(admin Admin, sid, family string, refreshExpires time.Time) (Tokens, error) {
	now := s.now().UTC()
	accessExp := now.Add(AccessTTL)
	a, err := s.signed(claims{Type: "access", AdminID: admin.ID, Username: admin.Username, SessionID: sid, FamilyID: family, RegisteredClaims: jwt.RegisteredClaims{Issuer: "ekkoplayer", Subject: strconv.FormatInt(admin.ID, 10), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(accessExp)}})
	if err != nil {
		return Tokens{}, err
	}
	r, err := s.signed(claims{Type: "refresh", AdminID: admin.ID, Username: admin.Username, SessionID: sid, FamilyID: family, RegisteredClaims: jwt.RegisteredClaims{Issuer: "ekkoplayer", Subject: strconv.FormatInt(admin.ID, 10), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(refreshExpires)}})
	if err != nil {
		return Tokens{}, err
	}
	return Tokens{Access: a, Refresh: r, AccessExpires: accessExp, RefreshExpiry: refreshExpires, Admin: admin}, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (Tokens, error) {
	var a Admin
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT id,username,password_hash FROM admins WHERE username=? COLLATE NOCASE AND active=1`, NormalizeUsername(username)).Scan(&a.ID, &a.Username, &hash)
	if err != nil || !VerifyPassword(hash, password) {
		return Tokens{}, ErrInvalidCredentials
	}
	sid, err := randomID()
	if err != nil {
		return Tokens{}, err
	}
	family, err := randomID()
	if err != nil {
		return Tokens{}, err
	}
	exp := s.now().UTC().Add(RefreshTTL)
	tok, err := s.issue(a, sid, family, exp)
	if err != nil {
		return Tokens{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO refresh_sessions(id,admin_id,family_id,token_hash,expires_at) VALUES(?,?,?,?,?)`, sid, a.ID, family, fingerprint(tok.Refresh), exp.Format(time.RFC3339Nano))
	return tok, err
}

func (s *Service) Access(raw string) (Admin, error) {
	c, err := s.parse(raw, "access")
	if err != nil {
		return Admin{}, err
	}
	return Admin{ID: c.AdminID, Username: c.Username}, nil
}

func (s *Service) Refresh(ctx context.Context, raw string) (Tokens, error) {
	c, err := s.parse(raw, "refresh")
	if err != nil {
		return Tokens{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Tokens{}, err
	}
	defer tx.Rollback()
	var hash, expires string
	var revoked sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT token_hash,expires_at,revoked_at FROM refresh_sessions WHERE id=? AND admin_id=? AND family_id=?`, c.SessionID, c.AdminID, c.FamilyID).Scan(&hash, &expires, &revoked)
	if err != nil || hash != fingerprint(raw) {
		return Tokens{}, ErrInvalidToken
	}
	if revoked.Valid {
		_, _ = tx.ExecContext(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,CURRENT_TIMESTAMP) WHERE family_id=?`, c.FamilyID)
		_ = tx.Commit()
		return Tokens{}, ErrRefreshReuse
	}
	exp, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !s.now().Before(exp) {
		return Tokens{}, ErrInvalidToken
	}
	newID, err := randomID()
	if err != nil {
		return Tokens{}, err
	}
	tok, err := s.issue(Admin{ID: c.AdminID, Username: c.Username}, newID, c.FamilyID, exp)
	if err != nil {
		return Tokens{}, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE refresh_sessions SET revoked_at=CURRENT_TIMESTAMP,replaced_by=?,last_used_at=CURRENT_TIMESTAMP WHERE id=? AND revoked_at IS NULL`, newID, c.SessionID)
	if err != nil {
		return Tokens{}, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return Tokens{}, ErrRefreshReuse
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO refresh_sessions(id,admin_id,family_id,token_hash,expires_at) VALUES(?,?,?,?,?)`, newID, c.AdminID, c.FamilyID, fingerprint(tok.Refresh), exp.Format(time.RFC3339Nano))
	if err != nil {
		return Tokens{}, err
	}
	if err = tx.Commit(); err != nil {
		return Tokens{}, err
	}
	return tok, nil
}

func (s *Service) Logout(ctx context.Context, raw string) error {
	c, err := s.parse(raw, "refresh")
	if err != nil {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,CURRENT_TIMESTAMP) WHERE id=?`, c.SessionID)
	return err
}
func (s *Service) LogoutAll(ctx context.Context, adminID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,CURRENT_TIMESTAMP) WHERE admin_id=?`, adminID)
	return err
}
func (s *Service) ChangePassword(ctx context.Context, adminID int64, current, next string) error {
	if len(next) < 12 || len(next) > 1024 {
		return errors.New("password must be 12..1024 characters")
	}
	var hash string
	if err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM admins WHERE id=? AND active=1`, adminID).Scan(&hash); err != nil || !VerifyPassword(hash, current) {
		return ErrInvalidCredentials
	}
	h, err := HashPassword(next)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE admins SET password_hash=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, h, adminID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,CURRENT_TIMESTAMP) WHERE admin_id=?`, adminID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) SecureCookies() bool { return s.secure }
