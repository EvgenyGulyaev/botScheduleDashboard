package validator

import (
	"regexp"
)

const emailRegex = `^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`

type UserEmailValidator struct {
	Email string
}

func (u UserEmailValidator) Validate() bool {
	re := regexp.MustCompile(emailRegex)
	return re.MatchString(u.Email)
}
