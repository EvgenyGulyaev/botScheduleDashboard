package middleware

import (
	"errors"
	"log"
	"net/http"
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

var instance *Jwt

var once sync.Once

func GetJwt() *Jwt {
	once.Do(func() {
		instance = initJwt(os.Getenv("JWT_KEY"))
	})
	return instance
}

type UserClaims struct {
	Username             string `json:"login"`
	jwt.RegisteredClaims        // Наследуемся от такой структуры
}

func (s *Jwt) Check(next func(c *silverlining.Context)) func(c *silverlining.Context) {
	return func(c *silverlining.Context) {
		tokenStr, err := GetToken(c)
		if err != nil {
			handleError(c, err.Error())
			return
		}

		token, err := jwt.ParseWithClaims(tokenStr, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return s.Key, nil
		})

		if err != nil || !token.Valid {
			handleError(c, err.Error())
			return
		}

		claims, ok := token.Claims.(*UserClaims)
		if !ok {
			handleError(c, err.Error())
			return
		}

		h := c.ResponseHeaders()
		h.Set("user", claims.Username)
		next(c)
	}
}

func (s *Jwt) CreateToken(login string) (string, error) {
	claims := UserClaims{
		Username: login,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.Key)
}

func GetToken(ctx *silverlining.Context) (string, error) {
	auth, isOk := ctx.RequestHeaders().Get("Authorization")
	if !isOk {
		return "", errors.New("authorization required")
	}

	parts := strings.SplitN(auth, " ", 2)

	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid Authorization format")
	}

	return parts[1], nil
}

func initJwt(k string) *Jwt {
	return &Jwt{Key: []byte(k)}
}

func handleError(ctx *silverlining.Context, value string) {
	err := ctx.WriteJSON(http.StatusUnauthorized, value)
	if err != nil {
		log.Print(err)
	}
}
