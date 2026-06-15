# plan: Task 3.16: Slideshow Video Trailer Stitching (`pw-mcp-viral`)

**Status:** Open (Issue #TBD)
**Go Version:** 1.26.4
**Date Completed:** —
**Unit Test Coverage:** —

Extend the `pw-mcp-viral` toolset with a slideshow generator tool that compiles sequences of paired images and audio narration segments into a single unified MP4 video trailer using ffmpeg.

## User Review Required

> [!NOTE]
> Requires standard local `ffmpeg` installation to process video concatenation and multiplexing.

## Proposed Changes

### MCP Server Tool Definition

#### [MODIFY] [main.go](file://../../cmd/pw-mcp-viral/main.go)
- Add `viral_stitch_slideshow` to the exposed tools in `setupServer`.
- Define JSON input schema:
  - `slides`: Array of objects, each containing:
    - `image_path` (string, required): Absolute path to the slide illustration.
    - `audio_path` (string, required): Absolute path to the voiceover narration for that slide.
  - `background_audio_path` (string, optional): Absolute path to background music track.
  - `output_name` (string, optional): Output MP4 filename.

### Video Stitching Logic

#### [MODIFY] [viral.go](file://../../internal/plugins/viral/viral.go)
- Add method `StitchSlideshow(ctx context.Context, slides []Slide, backgroundAudioPath, outputName string) (string, error)` to `ViralService`.
- For each slide:
  1. Render a temporary MP4 video segment from the static image looped over the audio track's duration:
     `ffmpeg -y -loop 1 -i <image_path> -i <audio_path> -c:v libx264 -tune stillimage -c:a aac -pix_fmt yuv420p -shortest <temp_segment_path>`
  2. Maintain a list of temporary segment file paths.
- Concatenate all temporary segments using the ffmpeg concat demuxer:
  - Generate a text file listing all temporary segment paths.
  - Run concat command:
    `ffmpeg -y -f concat -safe 0 -i <concat_list_path> -c copy <merged_video_path>`
- If `background_audio_path` is provided, mix/multiplex the background audio track into the merged video track using the ffmpeg amix filter.
- Clean up all temporary files on completion or error.

---

## Verification Plan

### Automated Tests
- Run `go test -v ./internal/plugins/viral/...`
- Test that the slideshow generation tool correctly parses inputs, handles errors, and executes the expected ffmpeg commands (using mock exec wrappers).

### Manual Verification
- Execute `pw-mcp-viral` and call the `viral_stitch_slideshow` tool with a list of test images and audio clips. Verify that the output is a fully synchronized slideshow video with narration.
