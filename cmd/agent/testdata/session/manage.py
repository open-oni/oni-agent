#!/bin/bash

if [[ $1 == "load_titles" ]]; then
    xml=$(cat $2/marc.xml)
    if [[ $xml == "<root>fail</root>" ]]; then
        echo "You asked for failure, bruh!"
        exit 1
    fi

    echo "Loading titles from XML: \"$xml\""
    exit 0
else
    echo "No!"
    exit 1
fi
