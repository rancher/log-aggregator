#!/bin/sh

echo "begging deploy...."
set -o errexit

VENDOR=rancher
DRIVER=rancherflex

# Assuming the single driver file is located at /$DRIVER inside the DaemonSet image.

driver_dir=$VENDOR${VENDOR:+"~"}${DRIVER}

echo "begging driver_dir is $driver_dir...."

if [ ! -d "/flexmnt/$driver_dir" ]; then
  mkdir -p "/flexmnt/$driver_dir"
fi

cp "/$DRIVER" "/flexmnt/$driver_dir/.$DRIVER"
mv -f "/flexmnt/$driver_dir/.$DRIVER" "/flexmnt/$driver_dir/$DRIVER"

while : ; do
  sleep 3600
done