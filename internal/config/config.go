package config

import (
	"os"
	"strings"
)

type Config struct {
	MongoURI    string
	DBName      string
	JWTSecret   []byte
	HTTPAddr    string
	AccessTTL   string // unused parsing simplify use constants in code
	RefreshTTL  string
	CORSOrigins []string
}

func Load() Config {
	uri := getenv("DISPATCH_MONGO_URI", "mongodb://localhost:27017")
	db := getenv("DISPATCH_DB", "dispatch")
	secret := getenv("DISPATCH_JWT_SECRET", "dev-secret-change-me")
	addr := getenv("DISPATCH_HTTP_ADDR", ":8080")
	cors := getenv("DISPATCH_CORS", "*")
	return Config{
		MongoURI:    uri,
		DBName:      db,
		JWTSecret:   []byte(secret),
		HTTPAddr:    addr,
		CORSOrigins: strings.Split(cors, ","),
	}
}

func getenv(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}
