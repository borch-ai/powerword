# plan: Task 3.7: Viral Promo Asset Builder Plugin (pw-mcp-viral)

**Status:** Open (Issue #TBD)

This task implements a native Go-based MCP server (`pw-mcp-viral`) that coordinates audio generation, video generation, and stitching using a local `ffmpeg` wrapper to generate short ASMR trailers for book parodies.

## User Review Required

> [!WARNING]
> This plugin relies on a local installation of the `ffmpeg` binary to handle video/audio multiplexing. We will implement checks to verify `ffmpeg` is present in the system's `$PATH` and gracefully report setup guides if it is missing.

## Proposed Changes

### Viral Plugin Component
Create a new directory `internal/plugins/viral/` to contain the media asset compiler.

#### [NEW] [viral.go](file:///Users/human/code/powerword/internal/plugins/viral/viral.go)
- Implement text-to-speech API bindings (ElevenLabs/OpenAI TTS).
- Implement video generator hooks (Veo/Sora or similar web service clients).
- Implement an executor wrapper for running local `ffmpeg` commands.
- Expose the following MCP tools:
  - `viral_generate_voiceover`: Synthesizes script narration using configured TTS providers.
  - `viral_generate_video`: Triggers generative video background tasks.
  - `viral_stitch_trailer`: Runs `ffmpeg` to merge background audio, voiceover tracks, and video clips into a vertical MP4 trailer.

#### [NEW] [viral_test.go](file:///Users/human/code/powerword/internal/plugins/viral/viral_test.go)
- Unit tests mocking API clients and checking command arguments constructed for `ffmpeg`.

### CLI Manifest Integration
#### [MODIFY] [internal/config/config.go](file:///Users/human/code/powerword/internal/config/config.go)
- Register the `pw-mcp-viral` server within the native plugin registry under the config key `[plugins.viral]`.

---

## Verification Plan

### Automated Tests
- Run `go test ./internal/plugins/viral/...` to ensure parameters are validated and `ffmpeg` execution path boundaries are guarded.
- Enforce the 91% unit test coverage requirement.

### Manual Verification
- Write a short parody script.
- Run `powerword "create a 15-second ASMR voiceover and stitch it with a background clip into a vertical video"` and verify the final MP4 output.
