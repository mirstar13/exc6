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
	// Group list and management
	router.Get("/groups", handlers.HandleGetGroups(gsrv))
	router.Post("/groups/create", handlers.HandleCreateGroup(gsrv))
	router.Delete("/groups/:groupId", handlers.HandleDeleteGroup(gsrv))

	// Group chat
	router.Get("/groups/:groupId/chat", handlers.HandleLoadGroupChat(csrv, gsrv, qdb))
	router.Post("/groups/:groupId/send", handlers.HandleSendGroupMessage(csrv, gsrv))
	router.Get("/groups/:groupId/sse", handlers.HandleGroupSSE(csrv, gsrv, qdb))

	// Group members
	router.Get("/groups/:groupId/members", handlers.HandleGroupMembers(gsrv))
	router.Post("/groups/:groupId/members", handlers.HandleAddGroupMember(gsrv))
	router.Delete("/groups/:groupId/members/:username", handlers.HandleRemoveGroupMember(gsrv))
}
