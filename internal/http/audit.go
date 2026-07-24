package httpapi

import (
	"context"
	"encoding/json"

	"github.com/openb00ks/openb00ks/internal/db"
)

func (hc *HandlerContext) auditEvent(ctx context.Context, entityID, actorID, objectType, objectID, action string, before, after interface{}) {
	if hc.audit == nil || entityID == "" {
		return
	}
	var beforeJSON []byte
	var afterJSON []byte
	if before != nil {
		beforeJSON, _ = json.Marshal(before)
	}
	if after != nil {
		afterJSON, _ = json.Marshal(after)
	}
	_ = hc.audit.Create(ctx, db.AuditEvent{
		EntityID:    entityID,
		ActorUserID: actorID,
		ObjectType:  objectType,
		ObjectID:    objectID,
		Action:      action,
		BeforeJSON:  beforeJSON,
		AfterJSON:   afterJSON,
	})
}
