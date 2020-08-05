package context

import (
	"context"
)

type key uint8

const (
	tenantIDKey key = iota
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
