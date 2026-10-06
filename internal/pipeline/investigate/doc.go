// Package investigate gathers the facts that prove an incident's root
// cause: crash output, node pressure, scheduling blockers, config
// changes, admission failures and image pull errors. It plans reads on
// the decision loop without I/O and runs them on the pipeline's pool
// under a deadline. It never imports the pipeline package.
package investigate
