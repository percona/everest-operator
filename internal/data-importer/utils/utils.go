package utils

import (
	"crypto/md5"
	"encoding/hex"
)

// GetMd5HashedName returns the first 8 characters of the MD5 hash of the input string.
// Used for generating unique object names that are not exposed directly to the user.
func GetMd5HashedName(s string) string {
	hash := md5.New()
	hash.Write([]byte(s))
	return hex.EncodeToString(hash.Sum(nil))[:8]
}
