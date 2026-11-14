#!/bin/sh
# Music: blacktrendmusic "Cyber" (short-02, 59 s) from ~/Downloads/cyber-2026-10-26-13-45-04-utc.zip
# BPM=110, MUSIC_AT=0, DROP=0 (same numbers as in scene.html)
cd "$(dirname "$0")"
node ~/.claude/skills/prodvid/scripts/mix.mjs picture.mp4 --out cyberviber.mp4 \
  --music source/track.wav --music-at 0 --fade-in 0.05 --fade-out 2.5 --level "0-43:0.72"
