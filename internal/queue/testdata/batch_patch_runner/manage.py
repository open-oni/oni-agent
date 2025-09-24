#!/bin/bash

if [[ $1 == "load_batch" ]]; then
    echo "Loading batch $2"
    exit 0
fi

if [[ $1 == "purge_batch" ]]; then
    echo "Purging batch $2"
    exit 0
fi

echo "Invalid request: $@"
exit 1
