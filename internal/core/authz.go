package core

// CanWriteArticle reports whether u may modify (update or delete) article a.
// Only the article's author may do so: access is granted exactly when
// u.ID equals a.UserID.
//
// It mirrors the source's AuthorizationService.canWriteArticle. Callers in
// the API layer turn a false result into a 403 (httpapi.NoAuthorization).
//
// MIGRATION_NOTE: Java would throw a NullPointerException for a null user or
// article. Here a nil argument means access is denied (returns false), so a
// caller bug can never grant access.
func CanWriteArticle(u *User, a *Article) bool {
	if u == nil || a == nil {
		return false
	}
	return u.ID == a.UserID
}

// CanWriteComment reports whether u may modify (delete) comment c on
// article a. Access is granted when u is the author of the article or the
// author of the comment.
//
// It mirrors the source's AuthorizationService.canWriteComment, including
// its short-circuit: the article author is allowed even before the comment
// is looked at.
//
// MIGRATION_NOTE: a nil user or article means access is denied. A nil
// comment still lets the article author through, just as Java's
// short-circuit || did, but nobody else.
func CanWriteComment(u *User, a *Article, c *Comment) bool {
	if u == nil || a == nil {
		return false
	}
	if u.ID == a.UserID {
		return true
	}
	return c != nil && u.ID == c.UserID()
}
