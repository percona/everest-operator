package webhooks

import "testing"

var validKeys = []string{
	"my-name",
	"example.com/my-name",
	"sub.domain.example.com/my-name",
	"sub-domain.example.com/my_name-1.0",
	"a/b",
	"a.b/c",
	"very-long-subdomain.another-very-long-subdomain.example.com/short-name",
}

var invalidKeys = []string{
	"example.com/",         // No name
	"/my-name",             // empty prefix is invalid according to the prefix requirements
	"example.com//my-name", // double slash
	"example..com/my-name", // consecutive dots in subdomain
	"example.com/MyName",   // invalid char
	"example.com/my-name_", // invalid char
	"example.com/my-name-", // ends with invalid char
	"example.com/-my-name", // starts with invalid char
	"toooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooolong.com/my-name", // subdomain too long
	"example.com/tooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooolong",  // name too long
	".example.com/my-name", // subdomain starts with invalid char
	"example.com./my-name", // subdomain ends with invalid char
	"-example.com/my-name", // subdomain starts with invalid char
	"example.com-/my-name", // subdomain ends with invalid char
}

func TestAnnotationKeyValidation(t *testing.T) {
	for _, key := range validKeys {
		if !annotationKeyRegex.MatchString(key) {
			t.Errorf("Key '%s' should be VALID, but it's INVALID", key)
		}
	}

	for _, key := range invalidKeys {
		if annotationKeyRegex.MatchString(key) {
			t.Errorf("Key '%s' should be INVALID, but it's VALID", key)
		}
	}
}
