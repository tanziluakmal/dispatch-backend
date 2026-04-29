package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"dispatch/backend/internal/model"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ImportPostman accepts a normalized collection payload (same shape as create) after client-side parsing.
func (s *Server) ImportPostman(c *gin.Context) {
	s.importCollectionPayload(c)
}

// ImportOpenAPI accepts a normalized collection payload from OpenAPI parsing on the client.
func (s *Server) ImportOpenAPI(c *gin.Context) {
	s.importCollectionPayload(c)
}

func (s *Server) importCollectionPayload(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var body struct {
		TeamID    string      `json:"teamId"`
		Name      string      `json:"name"`
		Variables interface{} `json:"variables"`
		Items     interface{} `json:"items"`
		Auth      interface{} `json:"auth"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	team, err := requireTeam(ctx, s.DB, body.TeamID, uid)
	if err != nil || !canWrite(roleOf(team, uid)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	col := model.APICollection{
		ID:        primitive.NewObjectID(),
		TeamID:    team.ID,
		Name:      strings.TrimSpace(body.Name),
		Variables: body.Variables,
		Items:     body.Items,
		Auth:      body.Auth,
		UpdatedAt: time.Now().UTC(),
		UpdatedBy: uid,
	}
	_, err = s.DB.Collection("collections").InsertOne(ctx, col)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"collections": []gin.H{collectionJSON(col)}})
}
