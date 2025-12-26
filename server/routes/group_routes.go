// fileName: mirstar13/exc6/exc6-main/server/routes/group_routes.go

package routes

import (
	"exc6/db"
	"exc6/server/handlers"
	"exc6/services/chat"
	"exc6/services/groups"

	"github.com/gofiber/fiber/v2"
)

// RegisterGroupRoutes sets up group-related endpoints
func RegisterGroupRoutes(router fiber.Router, qdb *db.Queries, csrv *chat.ChatService, gsrv *groups.GroupService) {
	// Group creation from dashboard
	router.Post("/groups/create", handlers.HandleCreateGroupFromDashboard(gsrv))

	// Group chat (integrated with dashboard)
	router.Get("/groups/:groupId/chat", handlers.HandleLoadGroupChatIntegrated(csrv, gsrv, qdb))
	router.Post("/groups/:groupId/send", handlers.HandleSendGroupMessage(csrv, gsrv))

	// Group members management
	router.Get("/groups/:groupId/members", handlers.HandleGroupMembersPartial(gsrv))
	router.Post("/groups/:groupId/members", handlers.HandleAddGroupMemberPartial(gsrv))
	router.Delete("/groups/:groupId/members/:username", handlers.HandleRemoveGroupMemberPartial(gsrv))

	// Group deletion
	router.Delete("/groups/:groupId", handlers.HandleDeleteGroupFromChat(gsrv))

	// Legacy: Keep the old groups page for backwards compatibility
	router.Get("/groups", handlers.HandleGetGroups(gsrv))
}
