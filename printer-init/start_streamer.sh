#!/bin/sh
LOGFILE=/mnt/UDISK/mjpg_streamer_boot.log

echo "[$(date '+%Y-%m-%d %H:%M:%S')] start_streamer.sh invoked" >> $LOGFILE

while [ ! -e /dev/video0 ]; do
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] waiting for /dev/video0..." >> $LOGFILE
    sleep 1
done

sleep 2
echo "[$(date '+%Y-%m-%d %H:%M:%S')] /dev/video0 found, launching mjpg_streamer" >> $LOGFILE

cd /mnt/UDISK
./mjpg_streamer -i "input_uvc.so -d /dev/video0 -r 640x480 -f 25" -o "output_http.so -p 8081 -w ./www" >> $LOGFILE 2>&1