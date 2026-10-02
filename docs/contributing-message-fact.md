# Add a message fact

A note is a few sentences. Each kind of fact has one small writer that returns
sentences or nothing. Real example: `expirySentences` in
`internal/notification/compose/proof.go`.

1. **Choose the part.** Sentences go in a part: `partLead`, `partProof`,
   `partConsequence`, `partUnverified`, `partRecurrence` or `partAction`
   (`note.go`). Parts are read in that order; within a part the heavier
   `weight` comes first. At most two proof sentences survive, so only
   evidence that convinces belongs in `partProof`.
2. **Write the writer.** A `sentenceWriter` is `func(caseFacts) []sentence`.
   Read only `caseFacts` (the incident, its members, the lead finding, the
   clock, redacted output and evidence). Return nil when the case has no such
   fact:

   ```go
   func expirySentences(f caseFacts) []sentence {
       if !f.ok { return nil }
       switch f.lead.Reason {
       case reasons.TLSCertExpired:
           return []sentence{{part: partConsequence,
               text: "Clients are rejecting it now."}}
       }
       return nil
   }
   ```

   Write plain English, one clause per sentence, no labels, links or
   bullets, and no emoji (the writer adds the single status emoji). Hedge
   with the confidence wording that already exists; do not invent certainty.
3. **Register it.** Add the writer to `noteWriters` in `note.go`. Updates and
   resolves use their own writers (`update.go`, `resolve.go`).
4. **Add a golden case.** Add a case to `goldenCases()` in
   `golden_cases_test.go` that builds an `incident.Incident` with the fact
   present, then generate and review the file:

   ```sh
   go test ./internal/notification/compose -run TestWriterGoldenNotes -update
   ```

   Read the new `testdata/<name>.golden` like code. Never update goldens to
   hide an unintended change.
5. **Run the property tests.** `TestWriterNoteProperties` fails on a label,
   a second emoji, a link, a line break or a repeated sentence.

```sh
go test ./internal/notification/compose
```

Escaping and redaction happen once, before the writer; evidence you read here
is already redacted. A provider with a short limit shows `Message.Short`, the
lead sentence, so keep the most important fact in the lead.
