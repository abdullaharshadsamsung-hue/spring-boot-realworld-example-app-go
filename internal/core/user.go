package core

import "fmt"

// FollowRelation records that the user identified by UserID follows the user
// identified by TargetID.
//
// It is a plain value object: the zero value (FollowRelation{}) is an empty
// relation with both fields unset, the equivalent of the source's no-arg
// constructor. Fields are exported, so they act as the getters/setters that
// Lombok's @Data generated. Every field is comparable, so == (and Equal)
// gives value-based equality over (UserID, TargetID), and values can be used
// directly as map keys in place of hashCode().
type FollowRelation struct {
	UserID   string `json:"userId"`
	TargetID string `json:"targetId"`
}

// NewFollowRelation creates a relation in which userID follows targetID.
func NewFollowRelation(userID, targetID string) FollowRelation {
	return FollowRelation{UserID: userID, TargetID: targetID}
}

// Equal reports whether r and other describe the same follower/target pair.
func (r FollowRelation) Equal(other FollowRelation) bool {
	return r == other
}

// String returns a representation including both field values, mirroring
// Lombok's generated toString.
func (r FollowRelation) String() string {
	return fmt.Sprintf("FollowRelation(userId=%s, targetId=%s)", r.UserID, r.TargetID)
}
