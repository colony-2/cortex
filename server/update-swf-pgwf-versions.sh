#!/bin/bash

# Define the packages to update
PACKAGES=(
    "github.com/colony-2/pgwf"
    "github.com/colony-2/pgwf-go"
    "github.com/colony-2/swf-go"
)

echo "Starting dependency updates..."

# Iterate through each subdirectory
for dir in */; do
    # Remove trailing slash for cleaner output
    dir=${dir%/}

    # Check if go.mod exists in the subdirectory
    if [[ -f "$dir/go.mod" ]]; then
        echo "----------------------------------------------------"
        echo "Processing project: $dir"
        
        # Enter the directory
        pushd "$dir" > /dev/null || continue

        # Flag to check if we found any of the packages in this project
        found_pkg=false

        for pkg in "${PACKAGES[@]}"; do
            # Check if the project actually depends on this package
            if go list -m "$pkg" > /dev/null 2>&1; then
                echo "Updating $pkg to latest..."
                go get -u "$pkg@latest"
                found_pkg=true
            fi
        done

        if [ "$found_pkg" = true ]; then
            echo "Running go mod tidy..."
            go mod tidy
        else
            echo "No matching packages found in this project."
        fi

        # Return to the parent directory
        popd > /dev/null || exit
    fi
done

echo "----------------------------------------------------"
echo "Done!"
