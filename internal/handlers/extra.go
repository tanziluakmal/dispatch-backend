package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dispatch/backend/internal/model"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (s *Server) GetTeam(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	team, err := requireTeam(ctx, s.DB, c.Param("teamId"), uid)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"team": teamJSON(team)})
}

func (s *Server) UpdateTeam(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	team, err := requireTeam(ctx, s.DB, c.Param("teamId"), uid)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	if roleOf(team, uid) != model.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "only owner can rename team"})
		return
	}
	team.Name = strings.TrimSpace(body.Name)
	_, err = s.DB.Collection("teams").ReplaceOne(ctx, bson.M{"_id": team.ID}, team)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"team": teamJSON(team)})
}

func (s *Server) DeleteTeam(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	team, err := requireTeam(ctx, s.DB, c.Param("teamId"), uid)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	if team.OwnerID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "only owner can delete"})
		return
	}
	_, _ = s.DB.Collection("collections").DeleteMany(ctx, bson.M{"team_id": team.ID})
	_, _ = s.DB.Collection("environments").DeleteMany(ctx, bson.M{"team_id": team.ID})
	if _, err := s.DB.Collection("teams").DeleteOne(ctx, bson.M{"_id": team.ID}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) RegenerateInvite(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	team, err := requireTeam(ctx, s.DB, c.Param("teamId"), uid)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	if roleOf(team, uid) != model.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "only owner can rotate invite"})
		return
	}
	code := randomCode(8)
	_, err = s.DB.Collection("teams").UpdateByID(ctx, team.ID, bson.M{"$set": bson.M{"invite_code": code}})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	team.InviteCode = code
	c.JSON(http.StatusOK, gin.H{"team": teamJSON(team)})
}

func (s *Server) GetCollection(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var col model.APICollection
	err = s.DB.Collection("collections").FindOne(ctx, bson.M{"_id": id}).Decode(&col)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if _, err := requireTeam(ctx, s.DB, col.TeamID.Hex(), uid); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"collection": collectionJSON(col)})
}

func (s *Server) DeleteCollection(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var existing model.APICollection
	err = s.DB.Collection("collections").FindOne(ctx, bson.M{"_id": id}).Decode(&existing)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	team, err := requireTeam(ctx, s.DB, existing.TeamID.Hex(), uid)
	if err != nil || !canWrite(roleOf(team, uid)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if _, err := s.DB.Collection("collections").DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) DeleteEnvironment(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var env model.Environment
	err = s.DB.Collection("environments").FindOne(ctx, bson.M{"_id": id}).Decode(&env)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	team, err := requireTeam(ctx, s.DB, env.TeamID.Hex(), uid)
	if err != nil || !canWrite(roleOf(team, uid)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if _, err := s.DB.Collection("environments").DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) ExportCollection(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var col model.APICollection
	err = s.DB.Collection("collections").FindOne(ctx, bson.M{"_id": id}).Decode(&col)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if _, err := requireTeam(ctx, s.DB, col.TeamID.Hex(), uid); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	payload := collectionJSON(col)
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	name := strings.ReplaceAll(strings.TrimSpace(col.Name), "/", "-")
	filename := "dispatch-collection-" + name + ".json"
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/json", b)
}

func parseExpectedAt(s *string) (time.Time, bool) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return time.Time{}, false
	}
	raw := strings.TrimSpace(*s)
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t, err = time.Parse(time.RFC3339, raw)
	}
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

func timesConflict(server time.Time, client time.Time) bool {
	st := server.UTC().Truncate(time.Millisecond)
	ct := client.UTC().Truncate(time.Millisecond)
	return !st.Equal(ct)
}

