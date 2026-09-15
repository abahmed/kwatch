package config

import _ "embed"

// providerCatalogData is the embedded guided-installer provider catalog.
//
// The TSV keeps provider metadata readable and editable as data instead of
// hiding it in a large Go string literal. The parser still accepts the
// version header and produces the same runtime catalog.
//
//go:embed provider_catalog.tsv
var providerCatalogData string
