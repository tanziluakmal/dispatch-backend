package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"dispatch/backend/internal/config"
	"dispatch/backend/internal/handlers"
	"dispatch/backend/internal/middleware"
	mongoclient "dispatch/backend/internal/mongo"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	cfg := config.Load()

	cli, err := mongoclient.Connect(cfg.MongoURI)
	if err != nil {
		log.Fatalf("mongo connect: %v", err)
	}
	defer func() {
		_ = cli.Disconnect(context.Background())
	}()

	db := cli.Database(cfg.DBName)
	if err := ensureIndexes(db); err != nil {
		log.Fatalf("indexes: %v", err)
	}

	srv := &handlers.Server{DB: db, Config: cfg}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(corsMiddleware())

	r.POST("/api/auth/register", srv.Register)
	r.POST("/api/auth/login", srv.Login)
	r.POST("/api/auth/refresh", srv.Refresh)

	api := r.Group("/api")
	api.Use(middleware.Auth(cfg.JWTSecret))

	api.GET("/teams", srv.ListTeams)
	api.POST("/teams", srv.CreateTeam)
	api.POST("/teams/join/:code", srv.JoinTeam)

	api.GET("/teams/:teamId/collections", srv.ListCollections)
	api.POST("/teams/:teamId/collections", srv.CreateCollection)
	api.GET("/teams/:teamId/environments", srv.ListEnvironments)
	api.POST("/teams/:teamId/environments", srv.CreateEnvironment)

	api.GET("/teams/:teamId", srv.GetTeam)
	api.PUT("/teams/:teamId", srv.UpdateTeam)
	api.DELETE("/teams/:teamId", srv.DeleteTeam)
	api.POST("/teams/:teamId/invite", srv.RegenerateInvite)

	api.GET("/collections/:id", srv.GetCollection)
	api.PUT("/collections/:id", srv.UpdateCollection)
	api.DELETE("/collections/:id", srv.DeleteCollection)

	api.PUT("/environments/:id", srv.UpdateEnvironment)
	api.DELETE("/environments/:id", srv.DeleteEnvironment)

	api.GET("/export/collection/:id", srv.ExportCollection)

	api.POST("/import/postman", srv.ImportPostman)
	api.POST("/import/openapi", srv.ImportOpenAPI)

	api.POST("/proxy", srv.ProxyRequest)

	addr := cfg.HTTPAddr
	if strings.TrimSpace(os.Getenv("PORT")) != "" && addr == ":8080" {
		addr = ":" + strings.TrimSpace(os.Getenv("PORT"))
	}

	log.Printf("dispatch backend listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func ensureIndexes(db *mongo.Database) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := db.Collection("users").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    map[string]int{"email": 1},
		Options: options.Index().SetUnique(true),
	})
	return err
}
