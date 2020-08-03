package context

import (
	"context"
)

type key uint8

const (
	tenantIDKey key = iota
	clientIPKey
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

func ClientIPFromContext(ctx context.Context) string {
	v := ctx.Value(ClientIPFromContext)
	if v == nil {
		return ""
	}
	return v.(string)
}

func ContextWithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey, ip)
}
