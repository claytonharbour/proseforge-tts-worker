# Changelog

## v0.3.0

### Fixed
- **RunPod heartbeat protocol** — heartbeat pings now include job IDs, matching the RunPod Python SDK protocol. Fixes jobs being marked as Failed despite successful processing (#15)
- **Authorization header** — removed "Bearer" prefix to match RunPod SDK format
- **Result POST** — appends `isStream=false` parameter matching SDK behavior
- **Abbreviation pauses** — periods in Dr., Mr., Mrs., etc. no longer trigger sentence splits (#17)
- **read/read homograph** — default flipped to past tense /rɛd/ for narrative prose; present tense only on modal verbs (will, can, to, etc.) (#17)
- **Acronym pronunciation** — DNA, EPA, FBI, etc. now pronounced as letter names ("dee en ay") instead of letter sounds (#18)
- **Year pronunciation** — "in 1987" now spoken as "nineteen eighty seven" instead of "one thousand nine hundred eighty seven" (#19)
- **lived/breathed** — corrected pronunciations via custom dictionary (#17)

### Added
- Diagnostic logging for RunPod result POST (payload size, timing, response body)
- Separate HTTP client for result POST (120s timeout vs 10s poll timeout)

## v0.2.0

### Fixed
- **RunPod heartbeat** — added heartbeat pings during job processing (#13)
- **Error logging** — result POST now logs response body on non-200 status (#13)

## v0.1.0

Initial public release.
- Kokoro-82M ONNX TTS with pure Go phonemization
- RunPod serverless worker with polling and heartbeat
- WAV and MP3 output
- Audio quality tools (analyze, diagnose, generate)
