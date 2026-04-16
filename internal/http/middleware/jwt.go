package middleware

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-www/silverlining"
	"github.com/golang-jwt/jwt/v5"
)

type Jwt struct {
	Key []byte
}

const (
	RefreshedTokenHeader = "X-Auth-Token"
	SessionDuration      = 7 * 24 * time.Hour
)

var instance *Jwt

var once sync.Once

func GetJwt() *Jwt {
	once.Do(func() {
		instance = initJwt(os.Getenv("JWT_KEY"))
	})
	return instance
}

type UserClaims struct {
	Email                string `json:"email"`
	Login                string `json:"login"`
	jwt.RegisteredClaims        // Наследуемся от такой структуры
}

func (s *Jwt) Check(next func(c *silverlining.Context)) func(c *silverlining.Context) {
	return func(c *silverlining.Context) {
		claims, err := s.getClaimsByToken(c)
		if err != nil {
			handleError(c, err.Error())
			return
		}
		if err := s.RefreshSession(c, claims.Email, claims.Login); err != nil {
			handleError(c, err.Error())
			return
		}

		h := c.ResponseHeaders()
		h.Set("user", claims.Email)

		next(c)
	}
}

func (s *Jwt) getEmailByToken(c *silverlining.Context) (string, error) {
	claims, err := s.getClaimsByToken(c)
	if err != nil {
		return "", err
	}
	return claims.Email, nil
}

func (s *Jwt) getClaimsByToken(c *silverlining.Context) (UserClaims, error) {
	tokenStr, err := GetToken(c)
	if err != nil {
		return UserClaims{}, err
	}

	claims, err := s.ValidateToken(tokenStr)
	if err != nil {
		return UserClaims{}, err
	}
	return claims, nil
}

func (s *Jwt) CreateToken(email, login string) (string, error) {
	claims := UserClaims{
		Email: email,
		Login: login,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(SessionDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.Key)
}

func (s *Jwt) RefreshSession(ctx *silverlining.Context, email, login string) error {
	token, err := s.CreateToken(email, login)
	if err != nil {
		return err
	}
	ctx.ResponseHeaders().Set(RefreshedTokenHeader, token)
	return nil
}

func GetToken(ctx *silverlining.Context) (string, error) {
	auth, isOk := ctx.RequestHeaders().Get("Authorization")
	if !isOk {
		return "", errors.New("authorization required")
	}

	return ParseBearerToken(auth)
}

func ParseBearerToken(auth string) (string, error) {
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid Authorization format")
	}
	return parts[1], nil
}

func (s *Jwt) ValidateToken(tokenStr string) (UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.Key, nil
	})

	if err != nil || !token.Valid {
		return UserClaims{}, errors.New("invalid token")
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok {
		return UserClaims{}, errors.New("invalid token")
	}
	return *claims, nil
}

func initJwt(k string) *Jwt {
	return &Jwt{Key: []byte(k)}
}
