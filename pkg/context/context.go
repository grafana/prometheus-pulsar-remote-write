package context

import (
	"context"
	"time"
)

type key uint8

const (
	tenantIDKey key = iota
	connectionStartTimeKey
)

func TenantIDFromContext(ctx context.Context) string {
	v := ctx.Value(tenantIDKey)
	if v == nil {
		return ""
	}
	return v.(string)
}

func ContextWithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

func ConnectionStartTimeFromContext(ctx context.Context) (t time.Time, ok bool) {
	v := ctx.Value(connectionStartTimeKey)
	if v == nil {
		return time.Time{}, false
	}
	return *v.(*time.Time), true
}

func ContextWithConnectionStartTime(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, connectionStartTimeKey, &t)
}
