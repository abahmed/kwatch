# Add a source, a kind schema or a watch mode

A source turns something it can see into `inventory.Observation`s. Everything
else reads the model. Real example: `LimitRangeSchema` in
`internal/inventory/kube/namespace.go`.

1. **Name the kind.** Add `KindLimitRange inventory.Kind = "limitrange"` (kinds
   live in `kinds.go` and next to their schema). The lower-case kind is what
   detectors and propagation rows match.
2. **Write the schema.** Implement `kube.Schema`: `Kind`, `RelationTypes`
   (every relation `Describe` can produce, so removed links are cleared),
   `Describe` (identity, attributes, relations; return false for foreign
   objects) and `Diff` (meaningful changes only; return nil for
   status-only updates). `LimitRangeSchema.Describe` records one attribute
   and a `Constrains` relation to the namespace. Add new relation types to
   `internal/inventory/relation.go` only when none fits.
3. **Register the informer.** Add a `registration{Resource, Schema, informer}`
   to the area function in `internal/inventory/kube/source.go`
   (`clusterRegistrations`). The registration also feeds `SourceAccess()`, so
   the RBAC audit follows, and `hashedResources` marks kinds whose cache keeps
   a data hash only. For a kind kwatch has no typed informer for, discovery
   watches it in `status` or `metadata` mode automatically; change
   `metadataResources` or the coverage entry in `coverage_catalog.go` to
   choose the mode.
4. **Check RBAC and docs.** The default ClusterRole already allows `list` and
   `watch` on every resource; the least-privilege role in
   `deploy/chart/templates/rbac.yaml` lists kinds by name, so add the new
   one there. Then regenerate the coverage page:
   `go run ./cmd/coveragedocs`.
5. **Test it.** Add the schema to `allSchemas()` in
   `schema_contract_test.go` (it rejects foreign objects and checks `Diff`),
   and a behavior test like `TestQuotaSchemaReportsExhaustedResources` in
   `policy_schema_test.go` that describes a real object and asserts
   attributes and relations.

```sh
go test ./internal/inventory/kube/... -run 'Schema|Coverage|Access'
make verify-focused PKGS="./internal/inventory/..."
```

A source that cannot read its resource makes the object unknown; it must never
create or resolve an incident. Keep clients, queues and cache sync inside the
source package; detectors only see the model.
