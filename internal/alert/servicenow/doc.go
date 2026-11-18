// Package servicenow keeps one ServiceNow incident per kwatch incident
// through the Table API: create once (found again by correlation id),
// add work notes on updates, resolve on recovery. It sends through the
// shared delivery transport.
package servicenow
