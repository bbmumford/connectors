# connectors

A provider-neutral abstraction for reading an external company directory (Microsoft Entra, Google Workspace, GitHub, or any other):

- a neutral `DirectoryGroup` and `DirectoryUser` model;
- one `DirectoryConnector` interface a consumer implements over a provider's API (`FetchGroups`, `FetchGroupMembers`, `FetchUsers`, `Provider`);
- a `Registry` that maps a provider name to a constructor, so this package never imports a provider SDK;
- `Sync`, which walks any connector into one reconciled `Snapshot` of groups and users with their memberships.

It ships no provider, no persistence and no mapping into an identity graph; those belong to the consumer.

## Use

```sh
go get github.com/bbmumford/connectors
```

```go
reg := connectors.NewRegistry()
reg.Register("entra", newEntraConnector) // a connectors.ConnectorFactory you write

c, err := reg.New("entra", connectors.ConnectorConfig{ /* tenant, credentials ... */ })
snap, err := connectors.Sync(ctx, c)
for _, g := range snap.Groups { /* ... */ }
```

An unknown provider name returns `ErrUnknownProvider`.

## Consumers

None yet.
