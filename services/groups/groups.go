package groups

import (
	"context"
	"database/sql"
	"exc6/apperrors"
	"exc6/db"
	"time"

	"github.com/google/uuid"
)

type GroupService struct {
	qdb *db.Queries
}

func NewGroupService(qdb *db.Queries) *GroupService {
	return &GroupService{qdb: qdb}
}

// GroupInfo represents a group with additional metadata
type GroupInfo struct {
	ID          string
	Name        string
	Description string
	Icon        string
	CustomIcon  string
	CreatedBy   string
	MemberCount int
	UserRole    string // Current user's role in the group
	CreatedAt   time.Time
}

// MemberInfo represents a group member
type MemberInfo struct {
	UserID     string
	Username   string
	Icon       string
	CustomIcon string
	Role       string
	JoinedAt   time.Time
}

// CreateGroup creates a new group with the creator as admin
func (gs *GroupService) CreateGroup(ctx context.Context, creatorUsername, name, description, icon string) (*GroupInfo, error) {
	// Get creator
	creator, err := gs.qdb.GetUserByUsername(ctx, creatorUsername)
	if err != nil {
		return nil, apperrors.NewUserNotFound()
	}

	// Validate group name
	if name == "" || len(name) < 3 {
		return nil, apperrors.NewValidationError("Group name must be at least 3 characters")
	}

	// Create group
	group, err := gs.qdb.CreateGroup(ctx, db.CreateGroupParams{
		Name:        name,
		Description: sql.NullString{String: description, Valid: description != ""},
		Icon:        sql.NullString{String: icon, Valid: icon != ""},
		CustomIcon:  sql.NullString{},
		CreatedBy:   creator.ID,
	})
	if err != nil {
		return nil, apperrors.NewDatabaseError("create group", err)
	}

	// Add creator as admin
	_, err = gs.qdb.AddGroupMember(ctx, db.AddGroupMemberParams{
		GroupID: group.ID,
		UserID:  creator.ID,
		Role:    "admin",
	})
	if err != nil {
		// Rollback - delete group
		gs.qdb.DeleteGroup(ctx, group.ID)
		return nil, apperrors.NewDatabaseError("add creator as admin", err)
	}

	return &GroupInfo{
		ID:          group.ID.String(),
		Name:        group.Name,
		Description: group.Description.String,
		Icon:        group.Icon.String,
		CustomIcon:  group.CustomIcon.String,
		CreatedBy:   creator.Username,
		MemberCount: 1,
		UserRole:    "admin",
		CreatedAt:   group.CreatedAt,
	}, nil
}

// GetUserGroups returns all groups a user is a member of
func (gs *GroupService) GetUserGroups(ctx context.Context, username string) ([]GroupInfo, error) {
	user, err := gs.qdb.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, apperrors.NewUserNotFound()
	}

	groups, err := gs.qdb.GetUserGroups(ctx, user.ID)
	if err != nil {
		return nil, apperrors.NewDatabaseError("get user groups", err)
	}

	result := make([]GroupInfo, 0, len(groups))
	for _, group := range groups {
		// Get member count
		count, err := gs.qdb.GetGroupMemberCount(ctx, group.ID)
		if err != nil {
			count = 0
		}

		// Get user's role
		member, err := gs.qdb.GetGroupMember(ctx, db.GetGroupMemberParams{
			GroupID: group.ID,
			UserID:  user.ID,
		})

		role := "member"
		if err == nil {
			role = member.Role
		}

		result = append(result, GroupInfo{
			ID:          group.ID.String(),
			Name:        group.Name,
			Description: group.Description.String,
			Icon:        group.Icon.String,
			CustomIcon:  group.CustomIcon.String,
			MemberCount: int(count),
			UserRole:    role,
			CreatedAt:   group.CreatedAt,
		})
	}

	return result, nil
}

// GetGroupInfo returns detailed information about a group
func (gs *GroupService) GetGroupInfo(ctx context.Context, groupID, username string) (*GroupInfo, error) {
	user, err := gs.qdb.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, apperrors.NewUserNotFound()
	}

	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return nil, apperrors.NewBadRequest("Invalid group ID")
	}

	// Check if user is member
	isMember, err := gs.qdb.IsGroupMember(ctx, db.IsGroupMemberParams{
		GroupID: groupUUID,
		UserID:  user.ID,
	})
	if err != nil || !isMember {
		return nil, apperrors.New(apperrors.ErrCodeUnauthorized, "Not a member of this group", 403)
	}

	group, err := gs.qdb.GetGroupByID(ctx, groupUUID)
	if err != nil {
		return nil, apperrors.NewDatabaseError("get group", err)
	}

	// Get member count
	count, _ := gs.qdb.GetGroupMemberCount(ctx, groupUUID)

	// Get user's role
	member, err := gs.qdb.GetGroupMember(ctx, db.GetGroupMemberParams{
		GroupID: groupUUID,
		UserID:  user.ID,
	})

	role := "member"
	if err == nil {
		role = member.Role
	}

	// Get creator username
	creator, _ := gs.qdb.GetUserByID(ctx, group.CreatedBy)
	creatorName := "Unknown"
	if creator.Username != "" {
		creatorName = creator.Username
	}

	return &GroupInfo{
		ID:          group.ID.String(),
		Name:        group.Name,
		Description: group.Description.String,
		Icon:        group.Icon.String,
		CustomIcon:  group.CustomIcon.String,
		CreatedBy:   creatorName,
		MemberCount: int(count),
		UserRole:    role,
		CreatedAt:   group.CreatedAt,
	}, nil
}

// GetGroupMembers returns all members of a group
func (gs *GroupService) GetGroupMembers(ctx context.Context, groupID, username string) ([]MemberInfo, error) {
	user, err := gs.qdb.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, apperrors.NewUserNotFound()
	}

	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return nil, apperrors.NewBadRequest("Invalid group ID")
	}

	// Check if user is member
	isMember, err := gs.qdb.IsGroupMember(ctx, db.IsGroupMemberParams{
		GroupID: groupUUID,
		UserID:  user.ID,
	})
	if err != nil || !isMember {
		return nil, apperrors.New(apperrors.ErrCodeUnauthorized, "Not a member of this group", 403)
	}

	members, err := gs.qdb.GetGroupMembers(ctx, groupUUID)
	if err != nil {
		return nil, apperrors.NewDatabaseError("get group members", err)
	}

	result := make([]MemberInfo, 0, len(members))
	for _, member := range members {
		result = append(result, MemberInfo{
			UserID:     member.ID.String(),
			Username:   member.Username,
			Icon:       member.Icon.String,
			CustomIcon: member.CustomIcon.String,
			Role:       member.Role,
			JoinedAt:   member.JoinedAt,
		})
	}

	return result, nil
}

// AddMember adds a user to a group (only admins can add)
func (gs *GroupService) AddMember(ctx context.Context, groupID, adderUsername, newMemberUsername string) error {
	// Get adder
	adder, err := gs.qdb.GetUserByUsername(ctx, adderUsername)
	if err != nil {
		return apperrors.NewUserNotFound()
	}

	// Get new member
	newMember, err := gs.qdb.GetUserByUsername(ctx, newMemberUsername)
	if err != nil {
		return apperrors.NewBadRequest("User not found")
	}

	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return apperrors.NewBadRequest("Invalid group ID")
	}

	// Check if adder is admin
	isAdmin, err := gs.qdb.IsGroupAdmin(ctx, db.IsGroupAdminParams{
		GroupID: groupUUID,
		UserID:  adder.ID,
	})
	if err != nil || !isAdmin {
		return apperrors.New(apperrors.ErrCodeUnauthorized, "Only admins can add members", 403)
	}

	// Check if user is already a member
	isMember, _ := gs.qdb.IsGroupMember(ctx, db.IsGroupMemberParams{
		GroupID: groupUUID,
		UserID:  newMember.ID,
	})
	if isMember {
		return apperrors.NewBadRequest("User is already a member")
	}

	// Add member
	_, err = gs.qdb.AddGroupMember(ctx, db.AddGroupMemberParams{
		GroupID: groupUUID,
		UserID:  newMember.ID,
		Role:    "member",
	})
	if err != nil {
		return apperrors.NewDatabaseError("add member", err)
	}

	return nil
}

// RemoveMember removes a user from a group
func (gs *GroupService) RemoveMember(ctx context.Context, groupID, removerUsername, targetUsername string) error {
	// Get remover
	remover, err := gs.qdb.GetUserByUsername(ctx, removerUsername)
	if err != nil {
		return apperrors.NewUserNotFound()
	}

	// Get target
	target, err := gs.qdb.GetUserByUsername(ctx, targetUsername)
	if err != nil {
		return apperrors.NewBadRequest("User not found")
	}

	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return apperrors.NewBadRequest("Invalid group ID")
	}

	// Check if remover is admin or removing themselves
	isAdmin, _ := gs.qdb.IsGroupAdmin(ctx, db.IsGroupAdminParams{
		GroupID: groupUUID,
		UserID:  remover.ID,
	})

	isSelf := remover.ID == target.ID

	if !isAdmin && !isSelf {
		return apperrors.New(apperrors.ErrCodeUnauthorized, "Only admins can remove members", 403)
	}

	// Remove member
	_, err = gs.qdb.RemoveGroupMember(ctx, db.RemoveGroupMemberParams{
		GroupID: groupUUID,
		UserID:  target.ID,
	})
	if err != nil {
		return apperrors.NewDatabaseError("remove member", err)
	}

	// If group is now empty, delete it
	count, _ := gs.qdb.GetGroupMemberCount(ctx, groupUUID)
	if count == 0 {
		gs.qdb.DeleteGroup(ctx, groupUUID)
	}

	return nil
}

// UpdateMemberRole updates a member's role (admin only)
func (gs *GroupService) UpdateMemberRole(ctx context.Context, groupID, updaterUsername, targetUsername, newRole string) error {
	if newRole != "admin" && newRole != "member" {
		return apperrors.NewValidationError("Role must be 'admin' or 'member'")
	}

	// Get updater
	updater, err := gs.qdb.GetUserByUsername(ctx, updaterUsername)
	if err != nil {
		return apperrors.NewUserNotFound()
	}

	// Get target
	target, err := gs.qdb.GetUserByUsername(ctx, targetUsername)
	if err != nil {
		return apperrors.NewBadRequest("User not found")
	}

	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return apperrors.NewBadRequest("Invalid group ID")
	}

	// Check if updater is admin
	isAdmin, err := gs.qdb.IsGroupAdmin(ctx, db.IsGroupAdminParams{
		GroupID: groupUUID,
		UserID:  updater.ID,
	})
	if err != nil || !isAdmin {
		return apperrors.New(apperrors.ErrCodeUnauthorized, "Only admins can change roles", 403)
	}

	// Update role
	_, err = gs.qdb.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{
		GroupID: groupUUID,
		UserID:  target.ID,
		Role:    newRole,
	})
	if err != nil {
		return apperrors.NewDatabaseError("update role", err)
	}

	return nil
}

// DeleteGroup deletes a group (admin only)
func (gs *GroupService) DeleteGroup(ctx context.Context, groupID, username string) error {
	user, err := gs.qdb.GetUserByUsername(ctx, username)
	if err != nil {
		return apperrors.NewUserNotFound()
	}

	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return apperrors.NewBadRequest("Invalid group ID")
	}

	// Check if user is admin
	isAdmin, err := gs.qdb.IsGroupAdmin(ctx, db.IsGroupAdminParams{
		GroupID: groupUUID,
		UserID:  user.ID,
	})
	if err != nil || !isAdmin {
		return apperrors.New(apperrors.ErrCodeUnauthorized, "Only admins can delete groups", 403)
	}

	// Delete group (CASCADE will remove members)
	_, err = gs.qdb.DeleteGroup(ctx, groupUUID)
	if err != nil {
		return apperrors.NewDatabaseError("delete group", err)
	}

	return nil
}
