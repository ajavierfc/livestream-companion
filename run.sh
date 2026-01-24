#!/bin/bash
cd $(dirname $0)
mkdir -p tmp bin
[ ! -f bin/ffmpeg ] && ln -s `which ffmpeg` bin/ffmpeg
[ $? -ne 0 ] && echo no ffmpeg installation found
./livestream-companion
