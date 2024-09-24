#!/bin/bash -x

CONTENT_PATH=$1
ROOT_PATH=$2

if [ ! -d "$CONTENT_PATH" ]; then
  exit 0
fi

# find all tar files recursively
for tarfile in $(find "$CONTENT_PATH" -name "*.tar" -type f)
do
  # try to import the tar file into containerd up to ten times
  for i in {1..10}
  do
    if [ "$ROOT_PATH" = "/" ]; then
      /opt/bin/ctr -n k8s.io image import "$tarfile" --all-platforms
    else
      "$ROOT_PATH"/opt/spectro/bin/ctr -n k8s.io --address /run/spectro/containerd/containerd.sock image import "$tarfile" --all-platforms
    fi
    if [ $? -eq 0 ]; then
      echo "Import successful: $tarfile (attempt $i)"
      break
    else
      if [ $i -eq 10 ]; then
        echo "Import failed: $tarfile (attempt $i)"
      fi
    fi
  done
done