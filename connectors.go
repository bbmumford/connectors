// Package connectors is the scope-agnostic directory-sync connector abstraction (extracted from the
// HSTLES identity auth-edge). It defines the neutral directory model (groups, users, membership) and a
// pluggable DirectoryConnector seam, plus a registry so a consumer registers provider constructors
// (Microsoft/Google/GitHub or any other) WITHOUT the neutral package importing their SDKs — the source's
// baked switch over provider names becomes data. A Sync helper walks any connector into a reconciled
// snapshot. Token refresh is a separate injected seam. This package pulls no HTTP/OAuth dependency; the
// concrete providers, with those deps, are the consumer's.
package connectors

import (
	"context"
	"fmt"
	"sync"
)

// DirectoryGroup is a group fetched from an external directory provider. ExternalID is the provider's
// opaque id; the package never interprets it.
type DirectoryGroup struct {
	ExternalID  string
	Name        string
	Email       string
	Description string
	MemberCount int
}

// DirectoryUser is a user fetched from an external directory provider. Groups holds the external group
// ids the user belongs to.
type DirectoryUser struct {
	ExternalID string
	Email      string
	Name       string
	Groups     []string
}

// ConnectorConfig holds the credentials a provider constructor needs. Fields are provider-interpreted;
// the registry passes the struct through unchanged.
type ConnectorConfig struct {
	ClientID     string
	ClientSecret string
	TenantID     string // e.g. an Azure AD tenant
	Domain       string // e.g. a Workspace domain
	AccessToken  string
	RefreshToken string
}

// DirectoryConnector fetches directory data from one external provider. A consumer implements it over the
// provider's API (the neutral package pulls no such dependency).
type DirectoryConnector interface {
	FetchGroups(ctx context.Context) ([]*DirectoryGroup, error)
	FetchGroupMembers(ctx context.Context, groupID string) ([]*DirectoryUser, error)
	FetchUsers(ctx context.Context) ([]*DirectoryUser, error)
	// Provider returns the opaque provider name the connector serves (e.g. "github").
	Provider() string
}

// ConnectorFactory builds a DirectoryConnector for a given credential config.
type ConnectorFactory func(config ConnectorConfig) (DirectoryConnector, error)

// TokenRefresher refreshes a provider's OAuth token and returns the new access token. It is an injected
// seam (the concrete per-provider refresh, with its OAuth dependency, is the consumer's); a provider
// whose tokens never expire returns the current token unchanged.
type TokenRefresher interface {
	RefreshToken(ctx context.Context, provider string, config ConnectorConfig) (accessToken string, err error)
}

// Registry maps an opaque provider name to a registered ConnectorFactory. It replaces the source's baked
// factory switch, so a consumer registers exactly the providers it supports without the neutral package
// depending on any of them. Concurrency-safe.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]ConnectorFactory
}

// NewRegistry returns an empty registry ready to register providers into.
func NewRegistry() *Registry { return &Registry{factories: map[string]ConnectorFactory{}} }

// Register adds or replaces the factory for a provider name.
func (r *Registry) Register(provider string, f ConnectorFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[provider] = f
}

// Providers returns the registered provider names.
func (r *Registry) Providers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for p := range r.factories {
		out = append(out, p)
	}
	return out
}

// ErrUnknownProvider is returned by New when no factory is registered for the provider.
type ErrUnknownProvider struct{ Provider string }

func (e *ErrUnknownProvider) Error() string {
	return fmt.Sprintf("connectors: unknown provider %q", e.Provider)
}

// New builds a connector for the provider via its registered factory. An unregistered provider returns
// *ErrUnknownProvider (fail-closed — never a silent default connector).
func (r *Registry) New(provider string, config ConnectorConfig) (DirectoryConnector, error) {
	r.mu.RLock()
	f, ok := r.factories[provider]
	r.mu.RUnlock()
	if !ok {
		return nil, &ErrUnknownProvider{Provider: provider}
	}
	return f(config)
}
