#!/bin/bash

if [[ $1 == "load_titles" ]]; then
    echo 'Loading titles from XML: "' $2 '"'
    exit 0
else
    echo "No!"
    exit 1
fi
