package db

import (
	"context"
	"database/sql"
)

type AuditStore struct {
	db *DB
}

func NewAuditStore(db *DB) *AuditStore {
	return &AuditStore{db: db}
}

type AuditEvent struct {
	EntityID      string
	ActorUserID   string
	ObjectType    string
	ObjectID      string
	Action        string
	BeforeJSON    []byte
	AfterJSON     []byte
	CorrelationID string
}

func (s *AuditStore) Create(ctx context.Context, event AuditEvent) error {
	var actorID sql.NullString
	if event.ActorUserID != "" {
		actorID = sql.NullString{String: event.ActorUserID, Valid: true}
	}
	var objectID sql.NullString
	if event.ObjectID != "" {
		objectID = sql.NullString{String: event.ObjectID, Valid: true}
	}
	var corrID sql.NullString
	if event.CorrelationID != "" {
		corrID = sql.NullString{String: event.CorrelationID, Valid: true}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_events (entity_id, actor_user_id, object_type, object_id, action, before_json, after_json, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, event.EntityID, actorID, event.ObjectType, objectID, event.Action, event.BeforeJSON, event.AfterJSON, corrID)
	return err
}
