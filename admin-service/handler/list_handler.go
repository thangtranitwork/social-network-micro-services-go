package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h *AdminHandler) GetPostsList(c *gin.Context) {
	skipStr := c.DefaultQuery("skip", "0")
	limitStr := c.DefaultQuery("limit", "20")
	skip, _ := strconv.Atoi(skipStr)
	limit, _ := strconv.Atoi(limitStr)

	ctx := c.Request.Context()
	posts, err := h.svc.GetPostsList(ctx, skip, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for i := range posts {
		isOnline, _ := h.svc.GetUserOnlineStatus(ctx, posts[i].Author.ID)
		posts[i].Author.IsOnline = isOnline
		posts[i].User.IsOnline = isOnline
	}

	sendSuccess(c, posts)
}

func (h *AdminHandler) GetUsersList(c *gin.Context) {
	skipStr := c.DefaultQuery("skip", "0")
	limitStr := c.DefaultQuery("limit", "20")
	skip, _ := strconv.Atoi(skipStr)
	limit, _ := strconv.Atoi(limitStr)

	ctx := c.Request.Context()
	users, err := h.svc.GetUsersList(ctx, skip, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for i := range users {
		isOnline, lastOnline := h.svc.GetUserOnlineStatus(ctx, users[i].ID)
		users[i].IsOnline = isOnline
		users[i].LastOnline = lastOnline
	}

	sendSuccess(c, users)
}
