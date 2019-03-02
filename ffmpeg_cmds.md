# Capture screenshot every 10 seconds and scale to 720p

`ffmpeg -i input.mp4 -vf "fps=1/10,scale=-2:720" ./frames/img%06d.jpg`

# Get video length in seconds

`ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 input.mp4`

