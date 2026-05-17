#!/bin/bash
cd $(dirname $0)
mkdir -p tmp bin
if [ ! -f bin/ffmpeg ]; then
  which ffmpeg && ln -s `which ffmpeg` bin/ffmpeg
  [ $? -ne 0 ] && echo no ffmpeg installation found && exit 1
fi
./livestream-companion