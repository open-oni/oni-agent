#!/bin/bash

if [[ $1 == "succeed" ]]; then
    echo "Yes!"
    exit 0
else
    echo "No!"
    exit 1
fi
