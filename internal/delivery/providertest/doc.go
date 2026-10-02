// Package providertest holds shared fixtures for provider tests: the three
// incident messages of one lifecycle (announce, update, resolve), an
// httptest recorder that captures what a provider sent, and assertions
// every text payload must pass. Production code never imports it.
package providertest
