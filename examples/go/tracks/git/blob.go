// Package git holds the example for the Git track: the object ID that
// Git gives the content of a file.
package git

import (
	"crypto/sha1"
	"encoding/hex"
	"strconv"
)

// region: blob

// BlobHash returns the object ID that Git gives a file with this
// content in a SHA-1 repository, the same ID that git hash-object
// prints. Git hashes a header, the word blob, a space, the length in
// decimal and a zero byte, followed by the content itself. The file
// name and permissions are not part of a blob: they live in the tree
// that lists it.
func BlobHash(content []byte) string {
	h := sha1.New()
	h.Write([]byte("blob " + strconv.Itoa(len(content)) + "\x00"))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// endregion: blob
