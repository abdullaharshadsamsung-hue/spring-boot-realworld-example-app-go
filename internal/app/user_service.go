package app

import "migrated-app/internal/core"

// UpdateUserCommand pairs the authenticated user being updated (the target
// user) with the requested changes. It is the unit of input to the user
// service's update operation.
//
// The command is immutable: fields are unexported and exposed only through
// accessors, mirroring the source's @Getter + @AllArgsConstructor class.
//
// MIGRATION_NOTE: the source carried a class-level @UpdateUserConstraint whose
// ConstraintValidator (UpdateUserValidator in UserService.java) checked that
// the new email/username are not already taken by a different user, triggered
// via @Valid on UserService.updateUser through Spring's method-validation AOP
// proxy. Go has no annotation-driven validation, so that check must be invoked
// explicitly by the user service's update method (it needs the user
// repository, so it cannot live on this plain value type). Call sites:
// CurrentUserApi PUT /user handler and GraphQL UserMutation.updateUser.
type UpdateUserCommand struct {
	targetUser *core.User
	param      UpdateUserParam
}

// NewUpdateUserCommand creates a command pairing targetUser with param. Like
// the source's all-args constructor it performs no validation and no
// transformation; validation happens separately when the command is handled.
func NewUpdateUserCommand(targetUser *core.User, param UpdateUserParam) UpdateUserCommand {
	return UpdateUserCommand{targetUser: targetUser, param: param}
}

// TargetUser returns the user passed to NewUpdateUserCommand.
func (c UpdateUserCommand) TargetUser() *core.User { return c.targetUser }

// Param returns the update parameters passed to NewUpdateUserCommand.
func (c UpdateUserCommand) Param() UpdateUserParam { return c.param }
