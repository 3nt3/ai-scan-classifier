#!/bin/sh

# This script runs main.go for all files in ~/Nextcloud/documents/scans
# and checks if the guess is correct. The guess should be equal to the file's parent folder name.

# Using fd

# Define the directory containing the files
SCAN_DIR=~/Nextcloud/Documents/scans

go build

# Iterate through each file in the directory
fd -e pdf --base-directory="$SCAN_DIR" | gshuf | while read -r file; do
    # Extract the parent folder name
    parent_folder=$(basename "$(dirname "$file")")

    # Run main.go and capture its output
    guess=$(./ai-scan-classifier "$SCAN_DIR/$file" | tail -n 1)

    # Check if the guess is correct
    if [ "$guess" = "$parent_folder" ]; then
        echo "Guess for $file is correct!"
    else
        echo "Incorrect guess for $file. Expected: $parent_folder, Actual: $guess"
    fi
done

