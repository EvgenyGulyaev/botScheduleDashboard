package middleware

import "github.com/go-www/silverlining"

type Hoc interface {
	Check(func(c *silverlining.Context)) func(c *silverlining.Context)
}
