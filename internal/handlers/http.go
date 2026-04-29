package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"dispatch/backend/internal/config"
	"dispatch/backend/internal/model"
	jwtutil "dispatch/backend/pkg/jwtutil"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

type Server struct {
	DB     *mongo.Database
	Config config.Config
}

func (s *Server) Register(c *gin.Context) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Email == "" || len(body.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email/password"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	u := model.User{
		ID:           primitive.NewObjectID(),
		Email:        body.Email,
		PasswordHash: string(hash),
		Name:         strings.TrimSpace(body.Name),
		CreatedAt:    time.Now().UTC(),
	}
	_, err = s.DB.Collection("users").InsertOne(ctx, u)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "email exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tokens, err := s.issueTokens(u.ID.Hex())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"accessToken":  tokens.Access,
		"refreshToken": tokens.Refresh,
		"user": gin.H{
			"id":    u.ID.Hex(),
			"email": u.Email,
		},
	})
}

func (s *Server) Login(c *gin.Context) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	var u model.User
	err := s.DB.Collection("users").FindOne(ctx, bson.M{"email": body.Email}).Decode(&u)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(body.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	tokens, err := s.issueTokens(u.ID.Hex())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"accessToken":  tokens.Access,
		"refreshToken": tokens.Refresh,
		"user": gin.H{
			"id":    u.ID.Hex(),
			"email": u.Email,
		},
	})
}

func (s *Server) Refresh(c *gin.Context) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cl, err := jwtutil.Parse(s.Config.JWTSecret, body.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh"})
		return
	}
	tokens, err := s.issueTokens(cl.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"accessToken": tokens.Access, "refreshToken": tokens.Refresh})
}

type tokenPair struct {
	Access  string
	Refresh string
}

func (s *Server) issueTokens(userHex string) (tokenPair, error) {
	access, err := jwtutil.SignAccess(s.Config.JWTSecret, userHex, 30*time.Minute)
	if err != nil {
		return tokenPair{}, err
	}
	refresh, err := jwtutil.SignAccess(s.Config.JWTSecret, userHex, 720*time.Hour)
	if err != nil {
		return tokenPair{}, err
	}
	return tokenPair{Access: access, Refresh: refresh}, nil
}

func userID(c *gin.Context) (primitive.ObjectID, error) {
	idStr, ok := c.Get("userID")
	if !ok {
		return primitive.NilObjectID, errors.New("no user")
	}
	return primitive.ObjectIDFromHex(idStr.(string))
}

func teamJSON(t model.Team) gin.H {
	members := make([]gin.H, 0, len(t.Members))
	for _, m := range t.Members {
		members = append(members, gin.H{"userId": m.UserID.Hex(), "role": string(m.Role)})
	}
	return gin.H{
		"id":         t.ID.Hex(),
		"name":       t.Name,
		"ownerId":    t.OwnerID.Hex(),
		"members":    members,
		"inviteCode": t.InviteCode,
	}
}

func roleOf(team model.Team, uid primitive.ObjectID) model.MemberRole {
	if team.OwnerID == uid {
		return model.RoleOwner
	}
	for _, m := range team.Members {
		if m.UserID == uid {
			return m.Role
		}
	}
	return ""
}

func requireTeam(ctx context.Context, db *mongo.Database, teamHex string, user primitive.ObjectID) (model.Team, error) {
	tid, err := primitive.ObjectIDFromHex(teamHex)
	if err != nil {
		return model.Team{}, err
	}
	var team model.Team
	err = db.Collection("teams").FindOne(ctx, bson.M{"_id": tid}).Decode(&team)
	if err != nil {
		return model.Team{}, err
	}
	if roleOf(team, user) == "" {
		return model.Team{}, errors.New("forbidden")
	}
	return team, nil
}

func canWrite(r model.MemberRole) bool {
	return r == model.RoleOwner || r == model.RoleEditor
}

func collectionJSON(col model.APICollection) gin.H {
	return gin.H{
		"id":        col.ID.Hex(),
		"teamId":    col.TeamID.Hex(),
		"name":      col.Name,
		"variables": col.Variables,
		"items":     col.Items,
		"auth":      col.Auth,
		"updatedAt": col.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func randomCode(n int) string {
	raw := make([]byte, (n+1)/2)
	_, _ = rand.Read(raw)
	s := strings.ToUpper(hex.EncodeToString(raw))
	if len(s) >= n {
		return s[:n]
	}
	return strings.Repeat("X", n)
}

func (s *Server) ListTeams(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	cur, err := s.DB.Collection("teams").Find(ctx, bson.M{
		"$or": []bson.M{
			{"owner_id": uid},
			{"members.user_id": uid},
		},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cur.Close(ctx)
	var teams []model.Team
	for cur.Next(ctx) {
		var t model.Team
		if err := cur.Decode(&t); err != nil {
			continue
		}
		teams = append(teams, t)
	}
	out := make([]gin.H, 0, len(teams))
	for _, t := range teams {
		out = append(out, teamJSON(t))
	}
	c.JSON(http.StatusOK, gin.H{"teams": out})
}

func (s *Server) CreateTeam(c *gin.Context) {
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
	t := model.Team{
		ID:         primitive.NewObjectID(),
		Name:       strings.TrimSpace(body.Name),
		OwnerID:    uid,
		Members:    []model.TeamMember{{UserID: uid, Role: model.RoleOwner}},
		InviteCode: randomCode(8),
		CreatedAt:  time.Now().UTC(),
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	_, err = s.DB.Collection("teams").InsertOne(ctx, t)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"team": teamJSON(t)})
}

func (s *Server) JoinTeam(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	code := strings.TrimSpace(strings.ToUpper(c.Param("code")))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	var t model.Team
	err = s.DB.Collection("teams").FindOne(ctx, bson.M{"invite_code": code}).Decode(&t)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "team not found"})
		return
	}
	for _, m := range t.Members {
		if m.UserID == uid {
			c.JSON(http.StatusOK, gin.H{"team": teamJSON(t)})
			return
		}
	}
	t.Members = append(t.Members, model.TeamMember{UserID: uid, Role: model.RoleEditor})
	_, err = s.DB.Collection("teams").UpdateByID(ctx, t.ID, bson.M{"$set": bson.M{"members": t.Members}})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"team": teamJSON(t)})
}

func (s *Server) ListCollections(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	team, err := requireTeam(ctx, s.DB, c.Param("teamId"), uid)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	cur, err := s.DB.Collection("collections").Find(ctx, bson.M{"team_id": team.ID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cur.Close(ctx)
	var rows []model.APICollection
	for cur.Next(ctx) {
		var col model.APICollection
		if err := cur.Decode(&col); err != nil {
			continue
		}
		rows = append(rows, col)
	}
	out := make([]gin.H, 0, len(rows))
	for _, col := range rows {
		out = append(out, collectionJSON(col))
	}
	c.JSON(http.StatusOK, gin.H{"collections": out})
}

func (s *Server) CreateCollection(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	team, err := requireTeam(ctx, s.DB, c.Param("teamId"), uid)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	if !canWrite(roleOf(team, uid)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "viewer cannot write"})
		return
	}
	var body struct {
		Name      string      `json:"name"`
		Variables interface{} `json:"variables"`
		Items     interface{} `json:"items"`
		Auth      interface{} `json:"auth"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	c.JSON(http.StatusCreated, gin.H{"collection": collectionJSON(col)})
}

func (s *Server) UpdateCollection(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
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
	var body struct {
		Name               string      `json:"name"`
		Variables          interface{} `json:"variables"`
		Items              interface{} `json:"items"`
		Auth               interface{} `json:"auth"`
		ExpectedUpdatedAt *string      `json:"expectedUpdatedAt,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if t, ok := parseExpectedAt(body.ExpectedUpdatedAt); ok {
		if timesConflict(existing.UpdatedAt, t) {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "collection": collectionJSON(existing)})
			return
		}
	}
	existing.Name = strings.TrimSpace(body.Name)
	existing.Variables = body.Variables
	existing.Items = body.Items
	existing.Auth = body.Auth
	existing.UpdatedAt = time.Now().UTC()
	existing.UpdatedBy = uid
	_, err = s.DB.Collection("collections").ReplaceOne(ctx, bson.M{"_id": id}, existing)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"collection": collectionJSON(existing)})
}

func (s *Server) ListEnvironments(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	team, err := requireTeam(ctx, s.DB, c.Param("teamId"), uid)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	cur, err := s.DB.Collection("environments").Find(ctx, bson.M{"team_id": team.ID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cur.Close(ctx)
	var rows []model.Environment
	for cur.Next(ctx) {
		var env model.Environment
		if err := cur.Decode(&env); err != nil {
			continue
		}
		rows = append(rows, env)
	}
	out := make([]gin.H, 0, len(rows))
	for _, env := range rows {
		out = append(out, gin.H{
			"id":        env.ID.Hex(),
			"name":      env.Name,
			"variables": env.Variables,
			"updatedAt": env.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	c.JSON(http.StatusOK, gin.H{"environments": out})
}

func (s *Server) CreateEnvironment(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	team, err := requireTeam(ctx, s.DB, c.Param("teamId"), uid)
	if err != nil || !canWrite(roleOf(team, uid)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var body struct {
		Name      string      `json:"name"`
		Variables interface{} `json:"variables"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	env := model.Environment{
		ID:        primitive.NewObjectID(),
		TeamID:    team.ID,
		Name:      strings.TrimSpace(body.Name),
		Variables: body.Variables,
		UpdatedAt: time.Now().UTC(),
	}
	_, err = s.DB.Collection("environments").InsertOne(ctx, env)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"environment": gin.H{
		"id": env.ID.Hex(), "name": env.Name, "variables": env.Variables,
		"updatedAt": env.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}})
}

func (s *Server) UpdateEnvironment(c *gin.Context) {
	uid, err := userID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
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
	var body struct {
		Name               string      `json:"name"`
		Variables          interface{} `json:"variables"`
		ExpectedUpdatedAt *string      `json:"expectedUpdatedAt,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if t, ok := parseExpectedAt(body.ExpectedUpdatedAt); ok {
		if timesConflict(env.UpdatedAt, t) {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "environment": gin.H{
				"id":        env.ID.Hex(),
				"name":      env.Name,
				"variables": env.Variables,
				"updatedAt": env.UpdatedAt.UTC().Format(time.RFC3339Nano),
			}})
			return
		}
	}
	env.Name = strings.TrimSpace(body.Name)
	env.Variables = body.Variables
	env.UpdatedAt = time.Now().UTC()
	_, err = s.DB.Collection("environments").ReplaceOne(ctx, bson.M{"_id": id}, env)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"environment": gin.H{
		"id": env.ID.Hex(), "name": env.Name, "variables": env.Variables,
		"updatedAt": env.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}})
}

