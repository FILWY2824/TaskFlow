package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTokenInvalid       = errors.New("token invalid")
	ErrTokenExpired       = errors.New("token expired")
)

// Claims access token 的 payload。
type Claims struct {
	UserID int64 `json:"uid"`
	jwt.RegisteredClaims
}

type Issuer struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	bcryptCost int
}

func NewIssuer(secret string, accessTTL, refreshTTL time.Duration, bcryptCost int) *Issuer {
	return &Issuer{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		bcryptCost: bcryptCost,
	}
}

func (i *Issuer) AccessTTL() time.Duration  { return i.accessTTL }
func (i *Issuer) RefreshTTL() time.Duration { return i.refreshTTL }

// IssueAccessToken 给用户签发新的 JWT access token。
func (i *Issuer) IssueAccessToken(userID int64, now time.Time) (string, time.Time, error) {
	exp := now.Add(i.accessTTL)
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			NotBefore: jwt.NewNumericDate(now),
			Subject:   fmt.Sprintf("%d", userID),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, exp, nil
}

// ParseAccessToken 校验并解码 access token。
func (i *Issuer) ParseAccessToken(raw string) (*Claims, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	tok, err := parser.ParseWithClaims(raw, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return i.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, ErrTokenInvalid
	}
	if claims.UserID <= 0 {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// IssueRefreshToken 生成新的不透明 refresh token(随机串),返回明文与哈希。
// 服务端只存哈希。
func (i *Issuer) IssueRefreshToken() (plain string, hash string, expiresAt time.Time, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", time.Time{}, fmt.Errorf("rand: %w", err)
	}
	plain = hex.EncodeToString(buf)
	hash = HashRefreshToken(plain)
	expiresAt = time.Now().Add(i.refreshTTL)
	return plain, hash, expiresAt, nil
}

// HashRefreshToken 用 SHA-256 计算 refresh token 的存储哈希。
// (refresh token 是高熵随机串,SHA-256 即可,不需要 bcrypt)
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// HashPassword 使用 bcrypt。
func (i *Issuer) HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), i.bcryptCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt: %w", err)
	}
	return string(h), nil
}

// CheckPassword 校验明文与哈希。
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
