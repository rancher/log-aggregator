#!/bin/sh

echo "Starting log-aggregator"
set -o errexit

VENDOR=cattle.io
DRIVER=log-aggregator

# Assuming the single driver file is located at /$DRIVER inside the DaemonSet image.

driver_dir=$VENDOR${VENDOR:+"~"}${DRIVER}

echo "driver_dir is $driver_dir"

if [ ! -d "/flexmnt/$driver_dir" ]; then
  mkdir -p "/flexmnt/$driver_dir"
fi

cp "/usr/bin/$DRIVER" "/flexmnt/$driver_dir/.$DRIVER"
mv -f "/flexmnt/$driver_dir/.$DRIVER" "/flexmnt/$driver_dir/$DRIVER"

while : ; do
  sleep 3600
done
