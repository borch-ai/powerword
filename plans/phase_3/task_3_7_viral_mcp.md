# plan: Task 3.7: Viral Promo Asset Builder Plugin (pw-mcp-viral)

**Status:** Completed
**Go Version:** 1.26
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 91%

This task implements a native Go-based MCP server (`pw-mcp-viral`) that coordinates audio generation, video generation, and stitching using a local `ffmpeg` wrapper to generate short ASMR trailers for book parodies.

## User Review Required

> [!WARNING]
> This plugin relies on a local installation of the `ffmpeg` binary to handle video/audio multiplexing. We will check if `ffmpeg` is present on the `$PATH` and report clear errors if it is missing.

## Proposed Changes

### Viral Plugin Component
Create a new directory `internal/plugins/viral/` to contain the media asset compiler.

#### [NEW] [viral.go](file:///Users/human/code/powerword/internal/plugins/viral/viral.go)
- [x] Implement text-to-speech API bindings (ElevenLabs/OpenAI TTS).
- [x] Implement video generator hooks (Veo/Sora or similar web service clients).
- [x] Implement an executor wrapper for running local `ffmpeg` commands.
- [x] Expose the following MCP tools:
  - `viral_generate_voiceover`: Synthesizes script narration using configured TTS providers.
  - `viral_generate_video`: Triggers generative video background tasks.
  - `viral_stitch_trailer`: Runs `ffmpeg` to merge background audio, voiceover tracks, and video clips into a vertical MP4 trailer.

#### [NEW] [viral_test.go](file:///Users/human/code/powerword/internal/plugins/viral/viral_test.go)
- [x] Unit tests mocking API clients and checking command arguments constructed for `ffmpeg`.

### CLI Manifest Integration
#### [MODIFY] [config.go](file:///Users/human/code/powerword/pkg/config/config.go)
- [x] Register the `pw-mcp-viral` server within the native plugin registry under the config key `[plugins.viral]`.

---

## Verification Plan

### Automated Tests
- [x] Run `go test ./internal/plugins/viral/...` to ensure parameters are validated and `ffmpeg` execution path boundaries are guarded.
- [x] Enforce the 91% unit test coverage requirement (achieved 91.2% overall internal package coverage).

### Manual Verification
- [x] Write a short parody script.
- [x] Run `powerword "create a 15-second ASMR voiceover and stitch it with a background clip into a vertical video"` and verify the final MP4 output.

## Implementation Details
- **Go version:** Go 1.26.4
- **CLI Plugin:** Exposes three MCP tools via stdio transport protocol.
- **Config:** Configurable via environment variables (e.g. `POWERWORD_VIRAL_TTS_PROVIDER`) and `powerword.toml`.
- **FFmpeg Stitching:** Mixes background music and voiceover audios dynamically using `amix` complex filter graph if background audio is provided.
- **Mock Backends:** Supports running in local mock environments using a temporary mock script mimicking ffmpeg executable, allowing fully self-contained unit tests.
