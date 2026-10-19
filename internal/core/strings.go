// Package core contains the domain model (users, articles, comments, tags)
// and small helpers shared by it.
package core

// isEmpty reports whether value has zero length. It is the Go counterpart of
// the source's Util.isEmpty(String): Go strings cannot be nil, so the zero
// value "" covers both the Java null and empty-string cases. Whitespace-only
// strings are intentionally NOT treated as empty, matching the source.
//
// It is used by the domain update methods (User.Update, Article.Update) to
// skip optional fields that the caller left blank.
func isEmpty(value string) bool {
	return value == ""
}
