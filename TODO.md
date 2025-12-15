# TODO

## FFmpeg / Relay

- [ ] **Allow streams with 0 video or audio tracks** - Currently ffmpeg arguments mandate both video and audio tracks exist. Need to handle streams that may not have a video track (e.g., radio channels) or audio track. The relay/transcoder should detect track availability and only include relevant stream mapping arguments.
