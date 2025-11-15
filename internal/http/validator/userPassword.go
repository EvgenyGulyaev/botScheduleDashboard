package validator

import "golang.org/x/crypto/bcrypt"

type UserPasswordValidator struct {
	Hash []byte
	Pass string
}

func (u UserPasswordValidator) Validate() bool {
	err := bcrypt.CompareHashAndPassword(u.Hash, []byte(u.Pass))
	return err == nil
}
