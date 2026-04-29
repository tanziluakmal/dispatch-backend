package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"`
	Email        string               `bson:"email"`
	PasswordHash string               `bson:"password_hash"`
	Name         string               `bson:"name"`
	CreatedAt    time.Time            `bson:"created_at"`
}

type MemberRole string

const (
	RoleOwner  MemberRole = "owner"
	RoleEditor MemberRole = "editor"
	RoleViewer MemberRole = "viewer"
)

type TeamMember struct {
	UserID primitive.ObjectID `bson:"user_id"`
	Role   MemberRole         `bson:"role"`
}

type Team struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	Name       string             `bson:"name"`
	OwnerID    primitive.ObjectID `bson:"owner_id"`
	Members    []TeamMember       `bson:"members"`
	InviteCode string             `bson:"invite_code"`
	CreatedAt  time.Time          `bson:"created_at"`
}

type APICollection struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	TeamID    primitive.ObjectID `bson:"team_id"`
	Name      string             `bson:"name"`
	Variables interface{}        `bson:"variables"`
	Items     interface{}        `bson:"items"`
	Auth      interface{}        `bson:"auth,omitempty"`
	UpdatedAt time.Time          `bson:"updated_at"`
	UpdatedBy primitive.ObjectID `bson:"updated_by,omitempty"`
}

type Environment struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	TeamID    primitive.ObjectID `bson:"team_id"`
	Name      string             `bson:"name"`
	Variables interface{}        `bson:"variables"`
	UpdatedAt time.Time          `bson:"updated_at"`
}
