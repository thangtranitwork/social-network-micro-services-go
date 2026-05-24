package validation

import (
	"regexp"
	"time"
	"unicode"
)

var (
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,36}$`)
)

func IsValidUsername(username string) bool {
	return usernameRegex.MatchString(username)
}

func IsValidAge(birthdate time.Time, minAge int) bool {
	today := time.Now()
	age := today.Year() - birthdate.Year()
	if today.YearDay() < birthdate.YearDay() {
		age--
	}
	return age >= minAge
}

func IsOnlyLettersAndSpaces(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
