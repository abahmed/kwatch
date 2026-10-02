// Package replay records the observations a pipeline receives and replays
// them later through a fresh engine on a simulated clock. It is test
// tooling: nothing in the running application imports it.
//
// # Recording
//
// Wrap the engine's Submit with a Recorder. Every observation is written
// as one JSON line before it reaches the engine:
//
//	var buf bytes.Buffer
//	rec, err := replay.NewRecorder(&buf, now, engine.Submit)
//	source := kube.NewTranslator(schema) // or any source
//	rec.Submit(ctx, source.Added(obj, true, now())...)
//
// The first line is a header naming the format and version and the time
// recording started:
//
//	{"format":"kwatch-observations","version":1,"start":"2026-09-29T14:00:00Z"}
//	{"at":"2026-09-29T14:00:00Z","obs":{"kind":"observed", ...}}
//
// A recorder needs no engine: pass nil as the next Submit to only write.
// Logs can also be built in code and written with Write.
//
// # Sanitizing
//
// A log from a real cluster names real workloads. Before sharing or
// committing one, pass it through a Sanitizer. Names, namespaces, UIDs,
// actors, GitOps applications, event sources, images and label values
// become stable hash-based pseudonyms, so the replay still links the same
// objects. Messages and other free text are dropped; reason codes such as
// CrashLoopBackOff are kept. A salt is required.
//
//	sanitizer, err := replay.NewSanitizer(replay.SanitizeOptions{Salt: s})
//	clean := sanitizer.Log(log)
//
// # Replaying
//
// Read the log and run it through an engine built from fresh dependencies:
//
//	log, err := replay.Read(file)
//	result, err := replay.Run(ctx, log, pipeline.Dependencies{
//		Model:     inventory.NewModel(inventory.Options{}),
//		Detectors: detection.NewRegistry(nil, detectors...),
//		Incidents: incident.NewManager(incident.Config{}, reasoner),
//	}, replay.Options{})
//
// Run owns the clock. It moves time only while the engine is idle, straight
// to the engine's next deadline (settle, recovery hold, recheck) or the next
// recorded observation, whichever comes first, and keeps ticking for
// Options.Tail after the last entry. Nothing waits on the wall clock, so
// the same log always yields the same decisions and messages, which the
// result returns in delivery order with the simulated time of each.
//
// The application tells the engine once every informer has synced. A
// replay does the same at Options.SyncAt, after the entries recorded up to
// then, so a cold start with failures that already existed goes through
// the startup summary:
//
//	result, err := replay.Run(ctx, log, deps, replay.Options{
//		SyncAt: log.Start.Add(30 * time.Second),
//	})
package replay
