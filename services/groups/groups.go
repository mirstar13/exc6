package groups

import (
	"context"
	"database/sql"
	"exc6/apperrors"
	"exc6/db"
	"exc6/pkg/breaker"
	"exc6/pkg/logger"
	"time"

	"github.com/google/uuid"
	"github.com/sony/gobreaker"
)

type GroupService struct {
	qdb *db.Queries
	cb  *gobreaker.CircuitBreaker
}

func NewGroupService(qdb *db.Queries) *GroupService {
	return &GroupService{
		qdb: qdb,
		cb: breaker.New(breaker.Config{
			Name:        "postgres-groups",
			MaxRequests: 10,
			Interval:    60 * time.Second,
			Timeout:     45 * time.Second,
			Threshold:   0.6,
			MinRequests: 10,
		}),
	}
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
	UserRole    string
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

// CreateGroup creates a new group with circuit breaker
func (gs *GroupService) CreateGroup(ctx context.Context, creatorUsername, name, description, icon string) (*GroupInfo, error) {
	// Validate group name
	if name == "" || len(name) < 3 {
		return nil, apperrors.NewValidationError("Group name must be at least 3 characters").
			WithOperation("group_creation").
			WithDetails("provided_name", name).
			WithDetails("min_length", 3).
			WithDetails("actual_length", len(name)).
			WithContext("creator", creatorUsername)
	}

	result, err := breaker.ExecuteCtx(ctx, gs.cb, func() (interface{}, error) {
		// Get creator
		creator, err := gs.qdb.GetUserByUsername(ctx, creatorUsername)
		if err != nil {
			return nil, apperrors.NewDatabaseQueryError(
				"SELECT * FROM users WHERE username = $1",
				creatorUsername,
				err,
			).WithOperation("group_creation_get_creator").
				WithContext("step", "fetching_creator")
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
			return nil, apperrors.NewDatabaseError("group_insert", err).
				WithOperation("group_creation").
				WithDetails("group_name", name).
				WithDetails("creator_id", creator.ID).
				WithContext("step", "inserting_group")
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

			return nil, apperrors.NewDatabaseError("group_member_insert", err).
				WithOperation("group_creation_add_admin").
				WithDetails("group_id", group.ID).
				WithDetails("group_name", name).
				WithDetails("creator_id", creator.ID).
				WithContext("step", "adding_creator_as_admin").
				WithContext("rollback", "group_deleted")
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
	})

	if err != nil {
		// Enrich with circuit breaker context
		if appErr, ok := err.(*apperrors.AppError); ok {
			appErr.WithContext("circuit_breaker_state", gs.cb.State().String())
			logger.WithFields(appErr.LogFields()).Error("Group creation failed")
			return nil, appErr
		}

		// Wrap unknown errors
		wrappedErr := apperrors.NewDatabaseError("group_creation", err).
			WithDetails("creator", creatorUsername).
			WithDetails("group_name", name).
			WithContext("circuit_breaker_state", gs.cb.State().String())

		logger.WithFields(wrappedErr.LogFields()).Error("Group creation failed")
		return nil, wrappedErr
	}

	return result.(*GroupInfo), nil
}

// GetUserGroups returns all groups a user is a member of
func (gs *GroupService) GetUserGroups(ctx context.Context, username string) ([]GroupInfo, error) {
	result, err := breaker.ExecuteCtx(ctx, gs.cb, func() (interface{}, error) {
		user, err := gs.qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
		}

		groups, err := gs.qdb.GetUserGroups(ctx, user.ID)
		if err != nil {
			return nil, err
		}

		infos := make([]GroupInfo, 0, len(groups))
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

			infos = append(infos, GroupInfo{
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

		return infos, nil
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"username": username,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to get user groups")
		return nil, apperrors.NewDatabaseError("get user groups", err)
	}

	return result.([]GroupInfo), nil
}

// GetGroupInfo returns detailed information about a group
func (gs *GroupService) GetGroupInfo(ctx context.Context, groupID, username string) (*GroupInfo, error) {
	result, err := breaker.ExecuteCtx(ctx, gs.cb, func() (interface{}, error) {
		user, err := gs.qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
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
			return nil, err
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
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"group_id": groupID,
			"username": username,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to get group info")
		return nil, err
	}

	return result.(*GroupInfo), nil
}

// GetGroupMembers returns all members of a group
func (gs *GroupService) GetGroupMembers(ctx context.Context, groupID, username string) ([]MemberInfo, error) {
	result, err := breaker.ExecuteCtx(ctx, gs.cb, func() (interface{}, error) {
		user, err := gs.qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
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
			return nil, err
		}

		infos := make([]MemberInfo, 0, len(members))
		for _, member := range members {
			infos = append(infos, MemberInfo{
				UserID:     member.ID.String(),
				Username:   member.Username,
				Icon:       member.Icon.String,
				CustomIcon: member.CustomIcon.String,
				Role:       member.Role,
				JoinedAt:   member.JoinedAt,
			})
		}

		return infos, nil
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"group_id": groupID,
			"username": username,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to get group members")
		return nil, err
	}

	return result.([]MemberInfo), nil
}

// AddMember adds a user to a group (only admins can add)
func (gs *GroupService) AddMember(ctx context.Context, groupID, adderUsername, newMemberUsername string) error {
	_, err := breaker.ExecuteCtx(ctx, gs.cb, func() (interface{}, error) {
		// Get adder
		adder, err := gs.qdb.GetUserByUsername(ctx, adderUsername)
		if err != nil {
			return nil, err
		}

		// Get new member
		newMember, err := gs.qdb.GetUserByUsername(ctx, newMemberUsername)
		if err != nil {
			return nil, apperrors.NewBadRequest("User not found")
		}

		groupUUID, err := uuid.Parse(groupID)
		if err != nil {
			return nil, apperrors.NewBadRequest("Invalid group ID")
		}

		// Check if adder is admin
		isAdmin, err := gs.qdb.IsGroupAdmin(ctx, db.IsGroupAdminParams{
			GroupID: groupUUID,
			UserID:  adder.ID,
		})
		if err != nil || !isAdmin {
			return nil, apperrors.New(apperrors.ErrCodeUnauthorized, "Only admins can add members", 403)
		}

		// Check if user is already a member
		isMember, _ := gs.qdb.IsGroupMember(ctx, db.IsGroupMemberParams{
			GroupID: groupUUID,
			UserID:  newMember.ID,
		})
		if isMember {
			return nil, apperrors.NewBadRequest("User is already a member")
		}

		// Add member
		_, err = gs.qdb.AddGroupMember(ctx, db.AddGroupMemberParams{
			GroupID: groupUUID,
			UserID:  newMember.ID,
			Role:    "member",
		})

		return nil, err
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"group_id":   groupID,
			"adder":      adderUsername,
			"new_member": newMemberUsername,
			"error":      err.Error(),
		}).Error("Circuit breaker: Failed to add member")
		return err
	}

	return nil
}

func (gs *GroupService) RemoveMember(ctx context.Context, groupID, removerUsername, targetUsername string) error {
	_, err := breaker.ExecuteCtx(ctx, gs.cb, func() (interface{}, error) {
		remover, err := gs.qdb.GetUserByUsername(ctx, removerUsername)
		if err != nil {
			return nil, err
		}

		targetUsername, err := gs.qdb.GetUserByUsername(ctx, targetUsername)
		if err != nil {
			return nil, apperrors.NewBadRequest("User not found")
		}

		groupUUID, err := uuid.Parse(groupID)
		if err != nil {
			return nil, apperrors.NewBadRequest("Invalid group ID")
		}

		// Check if remover is admin or removing themselves
		isAdmin, err := gs.qdb.IsGroupAdmin(ctx, db.IsGroupAdminParams{
			GroupID: groupUUID,
			UserID:  remover.ID,
		})
		if err != nil {
			return nil, err
		}

		isSelf := remover.ID == targetUsername.ID
		if !isAdmin && !isSelf {
			return nil, apperrors.New(apperrors.ErrCodeUnauthorized, "Only admins can remove members", 403)
		}

		// Remove member
		_, err = gs.qdb.RemoveGroupMember(ctx, db.RemoveGroupMemberParams{
			GroupID: groupUUID,
			UserID:  targetUsername.ID,
		})
		if err != nil {
			return nil, apperrors.NewDatabaseError("remove member", err)
		}

		// If group is now empty, delete it
		count, _ := gs.qdb.GetGroupMemberCount(ctx, groupUUID)
		if count == 0 {
			_, err := gs.qdb.DeleteGroup(ctx, groupUUID)
			if err != nil {
				return nil, apperrors.NewDatabaseError("delete empty group", err)
			}
		}

		return nil, nil
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"group_id":      groupID,
			"remover":       removerUsername,
			"target_member": targetUsername,
			"error":         err.Error(),
		}).Error("Circuit breaker: Failed to remove member")
		return err
	}

	return nil
}

func (gs *GroupService) UpdateMemberRole(ctx context.Context, groupID, updaterUsername, targetUsername, newRole string) error {
	_, err := breaker.ExecuteCtx(ctx, gs.cb, func() (interface{}, error) {
		if newRole != "admin" && newRole != "member" {
			return nil, apperrors.NewValidationError("Role must be 'admin' or 'member'")
		}

		// Get updater
		updater, err := gs.qdb.GetUserByUsername(ctx, updaterUsername)
		if err != nil {
			return nil, err
		}

		// Get target
		target, err := gs.qdb.GetUserByUsername(ctx, targetUsername)
		if err != nil {
			return nil, apperrors.NewBadRequest("User not found")
		}

		groupUUID, err := uuid.Parse(groupID)
		if err != nil {
			return nil, apperrors.NewBadRequest("Invalid group ID")
		}

		// Check if updater is admin
		isAdmin, err := gs.qdb.IsGroupAdmin(ctx, db.IsGroupAdminParams{
			GroupID: groupUUID,
			UserID:  updater.ID,
		})
		if err != nil || !isAdmin {
			return nil, apperrors.New(apperrors.ErrCodeUnauthorized, "Only admins can change roles", 403)
		}

		// Update role
		_, err = gs.qdb.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{
			GroupID: groupUUID,
			UserID:  target.ID,
			Role:    newRole,
		})
		if err != nil {
			return nil, apperrors.NewDatabaseError("update role", err)
		}

		return nil, nil
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"group_id":      groupID,
			"updater":       updaterUsername,
			"target_member": targetUsername,
			"new_role":      newRole,
			"error":         err.Error(),
		}).Error("Circuit breaker: Failed to update member role")
		return err
	}

	return nil
}

// DeleteGroup deletes a group (admin only)
func (gs *GroupService) DeleteGroup(ctx context.Context, groupID, username string) error {
	_, err := breaker.ExecuteCtx(ctx, gs.cb, func() (interface{}, error) {
		user, err := gs.qdb.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
		}

		groupUUID, err := uuid.Parse(groupID)
		if err != nil {
			return nil, apperrors.NewBadRequest("Invalid group ID")
		}

		// Check if user is admin
		isAdmin, err := gs.qdb.IsGroupAdmin(ctx, db.IsGroupAdminParams{
			GroupID: groupUUID,
			UserID:  user.ID,
		})
		if err != nil || !isAdmin {
			return nil, apperrors.New(apperrors.ErrCodeUnauthorized, "Only admins can delete groups", 403)
		}

		// Delete group (CASCADE will remove members)
		_, err = gs.qdb.DeleteGroup(ctx, groupUUID)
		return nil, err
	})

	if err != nil {
		logger.WithFields(map[string]interface{}{
			"group_id": groupID,
			"username": username,
			"error":    err.Error(),
		}).Error("Circuit breaker: Failed to delete group")
		return err
	}

	return nil
}

// GetMetrics returns circuit breaker metrics
func (gs *GroupService) GetMetrics() map[string]interface{} {
	state := gs.cb.State()
	counts := gs.cb.Counts()

	return map[string]interface{}{
		"state":                 state.String(),
		"total_requests":        counts.Requests,
		"total_successes":       counts.TotalSuccesses,
		"total_failures":        counts.TotalFailures,
		"consecutive_successes": counts.ConsecutiveSuccesses,
		"consecutive_failures":  counts.ConsecutiveFailures,
	}
}
